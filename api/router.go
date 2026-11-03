package api

import (
	"fmt"
	"html/template"
	"math/rand"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"context"
	"github.com/gorilla/mux"
	"shizukubot/events"

	"shizukubot/config"
	"shizukubot/core"
	"shizukubot/pluginregistry"
	"shizukubot/plugins"
	"shizukubot/rules"
	stats "shizukubot/storage/stats"
	"shizukubot/webhook"
	"shizukubot/webui"
)

var tempImages = sync.Map{}
var serverHost string

type tempImage struct {
	data      []byte
	createdAt time.Time
}

func init() {
	// Temporary images are transport resources, independent of plugin execution.

	go func() {
		for {
			time.Sleep(5 * time.Minute)
			now := time.Now()
			tempImages.Range(func(key, value interface{}) bool {
				img := value.(tempImage)
				if now.Sub(img.createdAt) > 10*time.Minute {
					tempImages.Delete(key)
				}
				return true
			})
		}
	}()
}

func SaveTempImage(data []byte) string {
	id := fmt.Sprintf("img_%d_%d", time.Now().UnixNano(), rand.Intn(100000))
	tempImages.Store(id, tempImage{data: data, createdAt: time.Now()})
	if serverHost != "" {
		return fmt.Sprintf("%s/temp/images/%s", serverHost, id)
	}
	return "/temp/images/" + id
}

func serveTempImage(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	if id == "" {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	value, ok := tempImages.Load(id)
	if !ok {
		http.Error(w, "image not found", http.StatusNotFound)
		return
	}

	img := value.(tempImage)
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(img.data)
}

type Router struct {
	*mux.Router
	botManager     *core.BotManager
	config         *config.Config
	configPath     string
	logManager     *core.LogManager
	templates      *template.Template
	plugins        *plugins.Manager
	rules          *rules.Engine
	configMu       sync.RWMutex
	configRevision string
	appearanceMu   sync.Mutex
	botOps         sync.Map
	pluginOps      sync.Map
}

func NewRouter(botManager *core.BotManager, cfg *config.Config, cfgPath string, logManager *core.LogManager) *Router {
	registry, err := pluginregistry.New()
	if err != nil {
		panic("compiled plugin registry initialization failed")
	}
	return NewRouterWithRegistry(botManager, cfg, cfgPath, logManager, registry)
}

// NewRouterWithRegistry supports embedded applications and isolated plugin tests.
func NewRouterWithRegistry(botManager *core.BotManager, cfg *config.Config, cfgPath string, logManager *core.LogManager, registry *plugins.Registry) *Router {
	cfg = cloneConfig(cfg)
	if cfg.WebUI.Appearance.Background.Type == "" {
		cfg.WebUI.Appearance = config.DefaultAppearance()
	}
	if cfg.Statistics.CleanupTime == "" {
		cfg.Statistics = stats.Defaults()
	}
	manager, err := plugins.NewManager(registry, botManager.Events(), "data/plugin-data")
	if err != nil {
		panic("plugin settings initialization failed")
	}
	manager.Skip = func(e events.Event) bool { bot, ok := botManager.GetBot(e.BotID); return ok && bot.MatchesBuiltin(e) }
	manager.Log = func(bot, id, msg string) { logManager.Error(bot, "plugin "+id+": "+msg) }

	if err := webui.InitUserStore("data/users.json"); err != nil {
		panic("failed to init user store: " + err.Error())
	}

	r := &Router{
		Router:         mux.NewRouter(),
		botManager:     botManager,
		config:         cfg,
		configRevision: newConfigRevision(),
		configPath:     cfgPath,
		logManager:     logManager,
		templates:      template.Must(template.ParseFiles("webui/templates/login.html")),
		plugins:        manager,
	}

	webui.ConfigureCookies(cfg.WebUI.SecureCookies)
	webui.InitSessionStore(cfg.WebUI.Password)

	botManager.SetCommandProvider(manager.Commands)
	botManager.Events().Subscribe(events.Filter{}, func(ctx context.Context, e events.Event) error {
		if bot, ok := botManager.GetBot(e.BotID); ok {
			if e.MessageID != "" {
				bot.Stats.IncReceived()
				bot.Stats.RecordGroup(e.GroupID)
				bot.Stats.RecordUser(e.UserID)
			}
			bot.HandleBuiltinCommand(ctx, e)
		}
		return nil
	})
	// Core commands keep their first subscription. Rules precede restored plugins.
	store, err := rules.NewStore("data/rules")
	if err != nil {
		_ = manager.Close(context.Background())
		panic("rule storage initialization failed")
	}
	r.rules = rules.NewEngine(store)
	r.rules.Skip = manager.Skip
	r.rules.OnError = func(bot, id, category string) { logManager.Error(bot, "rule "+id+": "+category) }
	if err = r.rules.Attach(botManager.Events()); err != nil {
		_ = r.rules.Close(context.Background())
		_ = manager.Close(context.Background())
		panic("rule event subscription failed")
	}
	bots := []string{}
	for _, bot := range botManager.GetAllBots() {
		bots = append(bots, bot.ID)
	}
	if err := manager.Restore(context.Background(), bots); err != nil {
		logManager.Error("", "one or more trusted plugin instances could not start")
	}

	r.registerRoutes()

	if cfg.WebUI.Host != "" {
		serverHost = cfg.WebUI.Host
	}

	return r
}

func (r *Router) registerRoutes() {
	r.HandleFunc("/api/appearance", r.PublicAppearance).Methods(http.MethodGet)
	r.HandleFunc("/appearance/bootstrap.js", r.AppearanceBootstrap).Methods(http.MethodGet)
	r.HandleFunc("/appearance/images/{file}", r.BackgroundImage).Methods(http.MethodGet)
	r.HandleFunc("/login", r.LoginPageHandler).Methods(http.MethodGet)
	r.HandleFunc("/login", webui.LoginHandler).Methods(http.MethodPost)
	r.HandleFunc("/logout", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusMethodNotAllowed) }).Methods(http.MethodGet)
	r.HandleFunc("/logout", webui.AuthMiddleware(http.HandlerFunc(webui.LogoutHandler)).ServeHTTP).Methods(http.MethodPost)

	r.PathPrefix("/static/").Handler(correctMIME(http.StripPrefix("/static/", http.FileServer(http.Dir("webui/static")))))
	r.HandleFunc("/temp/images/{id}", serveTempImage).Methods(http.MethodGet)

	r.HandleFunc("/webhook/{botID}", webhook.NewHandler(r.botManager).ServeHTTP).Methods(http.MethodPost)

	api := r.PathPrefix("/api").Subrouter()
	api.Use(webui.AdminMiddleware)
	// Only lifecycle/config edits for the same bot are serialized. Queries, sends,
	// sandbox execution and plugin operations never take this lock.
	api.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			parts := strings.Split(strings.Trim(req.URL.Path, "/"), "/")
			edit := len(parts) >= 3 && parts[0] == "api" && parts[1] == "bots" && ((len(parts) == 3 && (req.Method == "PUT" || req.Method == "DELETE")) || (req.Method == "POST" && len(parts) == 4 && (parts[3] == "start" || parts[3] == "stop")))
			if edit {
				v, _ := r.botOps.LoadOrStore(parts[2], &sync.Mutex{})
				lock := v.(*sync.Mutex)
				lock.Lock()
				defer lock.Unlock()
			}
			next.ServeHTTP(w, req)
		})
	})

	api.HandleFunc("/bots", r.GetBots).Methods(http.MethodGet)
	api.HandleFunc("/bots/{id}", r.GetBot).Methods(http.MethodGet)
	api.HandleFunc("/bots", r.CreateBot).Methods(http.MethodPost)
	api.HandleFunc("/bots/{id}", r.UpdateBot).Methods(http.MethodPut)
	api.HandleFunc("/bots/{id}", r.DeleteBot).Methods(http.MethodDelete)
	api.HandleFunc("/bots/{id}/message/group", r.SendGroupMessage).Methods(http.MethodPost)
	api.HandleFunc("/bots/{id}/message/c2c", r.SendC2CMessage).Methods(http.MethodPost)
	api.HandleFunc("/bots/{id}/start", r.StartBot).Methods(http.MethodPost)
	api.HandleFunc("/bots/{id}/stop", r.StopBot).Methods(http.MethodPost)
	api.HandleFunc("/bots/{id}/rules", r.GetRules).Methods(http.MethodGet)
	api.HandleFunc("/bots/{id}/rules", r.ReplaceRules).Methods(http.MethodPut)
	api.HandleFunc("/bots/{id}/rules/test", r.TestRules).Methods(http.MethodPost)
	api.HandleFunc("/stats/history", r.StatsHistory).Methods(http.MethodGet)
	api.HandleFunc("/stats/storage", r.StatsStatus).Methods(http.MethodGet)
	api.HandleFunc("/stats/cleanup", r.StatsCleanup).Methods(http.MethodPost)
	api.HandleFunc("/system", r.SystemInfo).Methods(http.MethodGet)
	api.HandleFunc("/appearance/background", r.UploadBackground).Methods(http.MethodPost)
	api.HandleFunc("/appearance/background", r.DeleteBackground).Methods(http.MethodDelete)
	api.HandleFunc("/stats", r.GetStats).Methods(http.MethodGet)
	api.HandleFunc("/bots/{id}/stats", r.GetBotStats).Methods(http.MethodGet)
	api.HandleFunc("/config", r.GetConfig).Methods(http.MethodGet)
	api.HandleFunc("/config", r.UpdateConfig).Methods(http.MethodPut)
	api.HandleFunc("/logs", r.GetLogs).Methods(http.MethodGet)

	api.HandleFunc("/users", r.GetUsers).Methods(http.MethodGet)
	api.HandleFunc("/users", r.CreateUser).Methods(http.MethodPost)
	api.HandleFunc("/users/{username}", r.DeleteUser).Methods(http.MethodDelete)
	api.HandleFunc("/users/{username}/password", r.ChangeUserPassword).Methods(http.MethodPut)
	api.HandleFunc("/user/me", r.GetCurrentUser).Methods(http.MethodGet)

	api.HandleFunc("/plugins", r.GetPlugins).Methods(http.MethodGet)
	api.HandleFunc("/plugins/{id}", r.GetPlugin).Methods(http.MethodGet)
	api.HandleFunc("/plugins", r.CreatePlugin).Methods(http.MethodPost)
	api.HandleFunc("/plugins/{id}", r.UpdatePlugin).Methods(http.MethodPut)
	api.HandleFunc("/plugins/{id}", r.DeletePlugin).Methods(http.MethodDelete)
	api.HandleFunc("/plugins/{id}/toggle", r.TogglePlugin).Methods(http.MethodPost)
	api.HandleFunc("/plugins/{id}/restart", r.RestartPlugin).Methods(http.MethodPost)
	api.HandleFunc("/plugins/{id}/export", r.ExportPlugin).Methods(http.MethodGet)
	api.HandleFunc("/plugins/data/restore", r.RestorePluginData).Methods(http.MethodPost)

	api.HandleFunc("/bots/{id}/sandbox/send", r.SandboxSend).Methods(http.MethodPost)
	api.HandleFunc("/bots/{id}/sandbox/plugins", r.SandboxPlugins).Methods(http.MethodGet)

	protected := r.PathPrefix("/").Subrouter()
	protected.HandleFunc("/", r.IndexHandler).Methods(http.MethodGet)
	protected.HandleFunc("/ws/logs", webui.LogsWSHandler(r.logManager)).Methods(http.MethodGet)
	protected.Use(webui.AuthMiddleware)
}

func (r *Router) LoginPageHandler(w http.ResponseWriter, rq *http.Request) {
	r.templates.ExecuteTemplate(w, "login.html", nil)
}

func (r *Router) IndexHandler(w http.ResponseWriter, rq *http.Request) {
	indexPath := filepath.Join("webui/templates", "index.html")
	http.ServeFile(w, rq, indexPath)
}

func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.Router.ServeHTTP(w, req)
}

func (r *Router) WithCORS() http.Handler {
	return webui.SecurityMiddleware(r)
}

// correctMIME 修正静态文件的 MIME 类型，解决 Windows 下 .css 被识别为 text/plain 的问题
// http.FileServer 内部会根据内容嗅探设置 Content-Type，可能在响应写出时覆盖预设值，
// 因此使用 ResponseWriter 包装器在最终写出时拦截并修正。
func correctMIME(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ext := strings.ToLower(filepath.Ext(r.URL.Path))
		correctType := ""
		if ext != "" {
			correctType = mime.TypeByExtension(ext)
		}
		if correctType == "" {
			next.ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(&contentTypeFixer{ResponseWriter: w, correctType: correctType}, r)
	})
}

type contentTypeFixer struct {
	http.ResponseWriter
	correctType string
	headerFixed bool
}

func (c *contentTypeFixer) WriteHeader(code int) {
	if !c.headerFixed {
		c.ResponseWriter.Header().Set("Content-Type", c.correctType)
		c.headerFixed = true
	}
	c.ResponseWriter.WriteHeader(code)
}

func (c *contentTypeFixer) Write(b []byte) (int, error) {
	if !c.headerFixed {
		c.ResponseWriter.Header().Set("Content-Type", c.correctType)
		c.headerFixed = true
	}
	return c.ResponseWriter.Write(b)
}
