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

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"

	"shizukubot/config"
	"shizukubot/core"
	"shizukubot/plugin"
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
	plugin.SaveTempImage = SaveTempImage

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
	botManager  *core.BotManager
	config      *config.Config
	configPath  string
	logManager  *core.LogManager
	templates   *template.Template
	pluginStore *plugin.PluginStore
	engine      *plugin.ExecutionEngine
}

func NewRouter(botManager *core.BotManager, cfg *config.Config, cfgPath string, logManager *core.LogManager) *Router {
	pluginStore, err := plugin.NewPluginStore("data/plugins")
	if err != nil {
		pluginStore = &plugin.PluginStore{
			Plugins: make(map[string]*plugin.Plugin),
		}
	}

	if err := webui.InitUserStore("data/users.json"); err != nil {
		panic("failed to init user store: " + err.Error())
	}

	r := &Router{
		Router:     mux.NewRouter(),
		botManager: botManager,
		config:     cfg,
		configPath: cfgPath,
		logManager: logManager,
		templates:  template.Must(template.ParseFiles("webui/templates/login.html")),
		pluginStore: pluginStore,
		engine:      plugin.NewExecutionEngine(pluginStore),
	}

	webui.InitSessionStore(cfg.WebUI.Password)

	r.registerRoutes()

	if cfg.WebUI.Host != "" {
		serverHost = cfg.WebUI.Host
	}

	return r
}

func (r *Router) GetEngine() *plugin.ExecutionEngine {
	return r.engine
}

func (r *Router) registerRoutes() {
	r.HandleFunc("/login", r.LoginPageHandler).Methods(http.MethodGet)
	r.HandleFunc("/login", webui.LoginHandler).Methods(http.MethodPost)
	r.HandleFunc("/logout", webui.LogoutHandler).Methods(http.MethodGet)

	r.PathPrefix("/static/").Handler(correctMIME(http.StripPrefix("/static/", http.FileServer(http.Dir("webui/static")))))
	r.PathPrefix("/litegraph/").Handler(correctMIME(http.StripPrefix("/litegraph/", http.FileServer(http.Dir("litegraph.js-release")))))
	r.HandleFunc("/temp/images/{id}", serveTempImage).Methods(http.MethodGet)

	r.HandleFunc("/webhook/{botID}", webhook.NewHandler(r.botManager).ServeHTTP).Methods(http.MethodPost)

	api := r.PathPrefix("/api").Subrouter()
	api.Use(webui.AuthMiddleware)
	api.HandleFunc("/bots", r.GetBots).Methods(http.MethodGet)
	api.HandleFunc("/bots/{id}", r.GetBot).Methods(http.MethodGet)
	api.HandleFunc("/bots", r.CreateBot).Methods(http.MethodPost)
	api.HandleFunc("/bots/{id}", r.UpdateBot).Methods(http.MethodPut)
	api.HandleFunc("/bots/{id}", r.DeleteBot).Methods(http.MethodDelete)
	api.HandleFunc("/bots/{id}/message/group", r.SendGroupMessage).Methods(http.MethodPost)
	api.HandleFunc("/bots/{id}/message/c2c", r.SendC2CMessage).Methods(http.MethodPost)
	api.HandleFunc("/bots/{id}/start", r.StartBot).Methods(http.MethodPost)
	api.HandleFunc("/bots/{id}/stop", r.StopBot).Methods(http.MethodPost)
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
	api.HandleFunc("/plugins/{id}/graph", r.SavePluginGraph).Methods(http.MethodPut)
	api.HandleFunc("/plugins/{id}/export", r.ExportPlugin).Methods(http.MethodGet)

	api.HandleFunc("/bots/{id}/sandbox/send", r.SandboxSend).Methods(http.MethodPost)
	api.HandleFunc("/bots/{id}/sandbox/plugins", r.SandboxPlugins).Methods(http.MethodGet)

	api.HandleFunc("/custom-nodes", r.GetCustomNodes).Methods(http.MethodGet)

	// 代码转换相关 API
	api.HandleFunc("/code/scan", r.ScanCode).Methods(http.MethodPost)
	api.HandleFunc("/code/code2graph", r.CodeToGraph).Methods(http.MethodPost)
	api.HandleFunc("/code/graph2code", r.GraphToCode).Methods(http.MethodPost)

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
	allowedOrigins := handlers.AllowedOrigins([]string{"*"})
	allowedMethods := handlers.AllowedMethods([]string{"GET", "POST", "PUT", "DELETE", "OPTIONS"})
	allowedHeaders := handlers.AllowedHeaders([]string{"Content-Type", "Authorization"})

	return handlers.CORS(allowedOrigins, allowedMethods, allowedHeaders)(r)
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
	correctType   string
	headerFixed   bool
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

func (r *Router) GetCustomNodes(w http.ResponseWriter, req *http.Request) {
	if plugin.GlobalCustomNodeStore == nil {
		SuccessResponse(w, []interface{}{})
		return
	}
	SuccessResponse(w, plugin.GlobalCustomNodeStore.List())
}