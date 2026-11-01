package api

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/gorilla/websocket"
	"net/http"
	"net/http/httptest"
	"os"
	"shizukubot/config"
	"shizukubot/core"
	"shizukubot/events"
	"shizukubot/pluginregistry"
	"shizukubot/plugins"
	"shizukubot/webui"
	"strings"
	"testing"
	"time"
)

func setupRouter(t *testing.T) (*Router, *http.Cookie) { return setupRouterWith(t) }
func setupRouterWith(t *testing.T, definitions ...plugins.Definition) (*Router, *http.Cookie) {
	t.Helper()
	login, err := os.ReadFile("../webui/templates/login.html")
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())
	os.MkdirAll("webui/templates", 0700)
	os.MkdirAll("config", 0700)
	os.WriteFile("webui/templates/login.html", login, 0600)
	os.WriteFile("webui/templates/index.html", []byte("<html>test</html>"), 0600)
	cfg := &config.Config{WebUI: config.WebUIConfig{Port: 4545, Password: "session-signing-secret-for-tests"}}
	bm := core.NewBotManager()
	registry, err := pluginregistry.New()
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range definitions {
		if err = registry.Register(d); err != nil {
			t.Fatal(err)
		}
	}
	r := NewRouterWithRegistry(bm, cfg, "config/config.json", bm.GetLogManager(), registry)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := r.Close(ctx); err != nil {
			t.Error(err)
		}
	})
	secret, err := os.ReadFile("data/.admin-initial-password")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]string{"username": "admin", "password": strings.TrimSpace(string(secret))})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.WithCORS().ServeHTTP(w, req)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"code":0`) {
		t.Fatal("login failed", w.Code, w.Body.String())
	}
	return r, w.Result().Cookies()[0]
}
func request(r *Router, cookie *http.Cookie, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if cookie != nil {
		req.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	r.WithCORS().ServeHTTP(w, req)
	return w
}
func TestAdminAPIAndSecretRedaction(t *testing.T) {
	r, c := setupRouter(t)
	if w := request(r, nil, "GET", "/api/config", ""); w.Code != 401 {
		t.Fatal("anonymous config", w.Code)
	}
	w := request(r, c, "GET", "/api/config", "")
	if w.Code != 200 || strings.Contains(w.Body.String(), r.config.WebUI.Password) {
		t.Fatal("secret exposed")
	}
	if err := webui.GetUserStore().CreateUser("viewer", "test-viewer-password", false); err != nil {
		t.Fatal(err)
	}
	viewer := request(r, nil, "POST", "/login", `{"username":"viewer","password":"test-viewer-password"}`)
	vc := viewer.Result().Cookies()[0]
	if response := request(r, vc, "GET", "/api/config", ""); response.Code != 403 {
		t.Fatal("nonadmin configuration access")
	}
	if response := request(r, vc, "GET", "/ws/logs", ""); response.Code != 403 {
		t.Fatal("nonadmin log access")
	}

	if c.SameSite != http.SameSiteLaxMode || !c.HttpOnly || c.Secure {
		t.Fatalf("local cookie policy: %+v", c)
	}
	req := httptest.NewRequest("POST", "/api/plugins", strings.NewReader(`{"name":"bad"}`))
	req.AddCookie(c)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://evil.invalid")
	denied := httptest.NewRecorder()
	r.WithCORS().ServeHTTP(denied, req)
	if denied.Code != 403 {
		t.Fatal("CSRF accepted")
	}
	for _, path := range []string{"/api/code/scan", "/api/code/code2graph"} {
		w := request(r, c, "POST", path, `{"source_type":"file","path":"/etc/passwd"}`)
		if strings.Contains(w.Body.String(), "root:") || strings.Contains(w.Body.String(), "/etc/passwd") {
			t.Fatal("file information leak", w.Body.String())
		}
	}
	if w := request(r, c, "GET", "/logout", ""); w.Code != 405 {
		t.Fatal("state changing GET logout")
	}
	if w := request(r, c, "POST", "/logout", `{}`); w.Code != 302 {
		t.Fatal("logout failed", w.Code)
	}
}
func TestSandboxSelectedBotAndNoLiveSending(t *testing.T) {
	r, c := setupRouterWith(t, testDefinition("failure", func() plugins.Extension {
		return &callbackExtension{command: "fail", handle: func(context.Context, plugins.Call) error { return errors.New("fixture failure") }}
	}))
	for _, typ := range []string{"group", "c2c"} {
		w := request(r, c, "POST", "/api/bots/selected/sandbox/send", `{"type":"`+typ+`","content":"/ping","plugin_id":"hello"}`)
		var result SandboxMessageResponse
		if json.Unmarshal(w.Body.Bytes(), &result) != nil || len(result.Replies) != 1 || result.Replies[0] != "pong" || len(result.Errors) != 0 || len(result.Executions) != 1 || result.Executions[0].Status != "completed" {
			t.Fatal(w.Body.String())
		}
	}
	w := request(r, c, "POST", "/api/bots/selected/sandbox/send", `{"content":"/fail","plugin_id":"failure"}`)
	var out SandboxMessageResponse
	json.Unmarshal(w.Body.Bytes(), &out)
	if len(out.Errors) != 1 || len(out.Executions) != 1 || out.Executions[0].Status != "failed" {
		t.Fatal("sandbox hid failure", w.Body.String())
	}
}
func TestLogWebsocketAuthorizationAndOrigin(t *testing.T) {
	r, c := setupRouter(t)
	s := httptest.NewServer(r.WithCORS())
	defer s.Close()
	url := "ws" + strings.TrimPrefix(s.URL, "http") + "/ws/logs"
	h := http.Header{"Origin": []string{"http://evil.invalid"}, "Cookie": []string{c.String()}}
	conn, resp, err := websocket.DefaultDialer.Dial(url, h)
	if conn != nil {
		conn.Close()
	}
	if err == nil || resp == nil || resp.StatusCode != 403 {
		t.Fatal("foreign WS origin accepted")
	}
	resp.Body.Close()
	conn, resp, err = websocket.DefaultDialer.Dial(url, nil)
	if conn != nil {
		conn.Close()
	}
	if err == nil {
		t.Fatal("anonymous websocket accepted")
	}
	if resp != nil {
		resp.Body.Close()
	}
	h.Del("Origin")
	conn, resp, err = websocket.DefaultDialer.Dial(url, h)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if resp != nil && resp.Body != nil {
		defer resp.Body.Close()
	}
	r.logManager.Info("test", "test stream record")
	conn.SetReadDeadline(time.Now().Add(time.Second))
	_, raw, err := conn.ReadMessage()
	if err != nil || len(raw) == 0 {
		t.Fatal("log stream unavailable", err)
	}
}
func TestScanConfinement(t *testing.T) {
	r, c := setupRouter(t)
	for _, test := range []struct{ method, path string }{{"POST", "/api/code/scan"}, {"POST", "/api/code/code2graph"}, {"POST", "/api/code/graph2code"}, {"PUT", "/api/plugins/test/graph"}, {"GET", "/api/custom-nodes"}} {
		w := request(r, c, test.method, test.path, `{"path":"/etc/passwd"}`)
		if w.Code != 404 {
			t.Fatal("retired execution route not absent", test, w.Code)
		}
	}

	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	w, _ := z.Create("../escape")
	w.Write([]byte("bad"))
	z.Close()
	raw, _ := json.Marshal(map[string]string{"package_base64": base64.StdEncoding.EncodeToString(buf.Bytes())})
	response := request(r, c, "POST", "/api/plugins", string(raw))
	if strings.Contains(response.Body.String(), `"code":0`) {
		t.Fatal("unsafe upload accepted")
	}
}

func TestPasswordChangeRevokesSession(t *testing.T) {
	r, c := setupRouter(t)
	if err := webui.GetUserStore().ChangePassword("admin", "changed-test-password"); err != nil {
		t.Fatal(err)
	}
	if w := request(r, c, "GET", "/api/config", ""); w.Code != 401 {
		t.Fatal("old cookie survived password change", w.Code)
	}
}

// Long plugin execution must not hold the configuration lock or block ordinary queries.
func TestLongSandboxDoesNotBlockQueries(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	r, c := setupRouterWith(t, testDefinition("blocking", func() plugins.Extension { return &blockingExtension{entered: entered, release: release} }))
	done := make(chan struct{})
	go func() {
		defer close(done)
		request(r, c, "POST", "/api/bots/chosen/sandbox/send", `{"content":"/wait","plugin_id":"blocking"}`)
	}()
	<-entered
	queried := make(chan int, 1)
	go func() { queried <- request(r, c, "GET", "/api/stats", "").Code }()
	select {
	case status := <-queried:
		if status != 200 {
			t.Fatal(status)
		}
	case <-time.After(time.Second):
		close(release)
		t.Fatal("sandbox blocked query")
	}
	close(release)
	<-done
}

type blockingExtension struct{ entered, release chan struct{} }

func (b *blockingExtension) Load(_ context.Context, h plugins.Host) error {
	return h.Command("wait", func(ctx context.Context, _ plugins.Call) error {
		close(b.entered)
		select {
		case <-b.release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
}
func (*blockingExtension) Start(context.Context) error                     { return nil }
func (*blockingExtension) HandleEvent(context.Context, plugins.Call) error { return nil }
func (*blockingExtension) Stop(context.Context) error                      { return nil }
func (*blockingExtension) Unload(context.Context) error                    { return nil }
func TestSandboxCapturesAllMedia(t *testing.T) {
	r, c := setupRouterWith(t, testDefinition("media", func() plugins.Extension {
		return &callbackExtension{command: "ping", handle: func(ctx context.Context, c plugins.Call) error {
			for _, kind := range []string{"text", "image", "rich_media", "markdown", "keyboard", "video"} {
				if err := c.Send(ctx, events.Reply{Kind: kind, Text: "text", URL: "https://example.com/media"}); err != nil {
					return err
				}
			}
			return nil
		}}
	}))
	w := request(r, c, "POST", "/api/bots/chosen/sandbox/send", `{"content":"/ping","plugin_id":"media"}`)
	var result SandboxMessageResponse
	if json.Unmarshal(w.Body.Bytes(), &result) != nil || len(result.Results) != 6 || len(result.Errors) != 0 {
		t.Fatal(w.Body.String())
	}
}

func TestCoreBuiltinCommandsKeepPriority(t *testing.T) {
	r, _ := setupRouterWith(t, testDefinition("listener", func() plugins.Extension {
		return &callbackExtension{event: "*", handle: func(ctx context.Context, c plugins.Call) error { return c.Reply(ctx, "plugin") }}
	}))
	bot, err := core.NewBot(config.BotConfig{ID: "builtin-test", AppID: "mock", AppSecret: "mock", Mode: "webhook"})
	if err != nil {
		t.Fatal(err)
	}
	if err = r.botManager.AddBot(bot); err != nil {
		t.Fatal(err)
	}
	if err = r.plugins.Enable(context.Background(), "listener", bot.ID); err != nil {
		t.Fatal(err)
	}
	var replies []string
	err = r.botManager.Events().Publish(context.Background(), events.Event{BotID: bot.ID, Type: "c2c_message", Message: "/help", Reply: func(_ context.Context, reply events.Reply) error { replies = append(replies, reply.Text); return nil }})
	if err != nil || len(replies) != 1 || replies[0] == "plugin" {
		t.Fatal("builtin priority regressed", replies, err)
	}
}

type callbackExtension struct {
	plugins.Base
	command string
	event   events.Type
	handle  func(context.Context, plugins.Call) error
}

func (e *callbackExtension) Load(_ context.Context, h plugins.Host) error {
	if e.command != "" {
		return h.Command(e.command, e.handle)
	}
	return h.On(e.event, e.handle)
}
func testDefinition(id string, create func() plugins.Extension) plugins.Definition {
	return plugins.Definition{Manifest: plugins.Manifest{APIVersion: 1, ID: id, Name: id, Version: "1.0.0", Runtime: "go", Permissions: []string{"commands.register", "events.subscribe", "message.send"}, ConfigSchema: plugins.Schema{Type: "object"}}, New: create}
}
func TestCompiledPluginManagement(t *testing.T) {
	r, c := setupRouter(t)
	for _, tc := range []struct{ method, path, body string }{{"POST", "/api/plugins", `{"package_base64":"ignored"}`}, {"DELETE", "/api/plugins/hello", `{}`}} {
		if w := request(r, c, tc.method, tc.path, tc.body); w.Code != http.StatusGone {
			t.Fatal(tc, w.Code)
		}
	}
	w := request(r, c, "GET", "/api/plugins", "")
	if !strings.Contains(w.Body.String(), `"id":"hello"`) {
		t.Fatal(w.Body.String())
	}
	if w = request(r, c, "PUT", "/api/plugins/counter", `{"bot_id":"selected","config":{"prefix":"test"}}`); !strings.Contains(w.Body.String(), `"code":0`) {
		t.Fatal(w.Body.String())
	}
	if w = request(r, c, "POST", "/api/plugins/counter/toggle", `{"bot_id":"selected","enabled":true}`); !strings.Contains(w.Body.String(), `"code":0`) {
		t.Fatal(w.Body.String())
	}
	if w = request(r, c, "POST", "/api/plugins/counter/restart", `{"bot_id":"selected"}`); !strings.Contains(w.Body.String(), `"code":0`) {
		t.Fatal(w.Body.String())
	}
	if w = request(r, c, "GET", "/api/plugins/counter/export", ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"plugin_id": "counter"`) || strings.Contains(w.Body.String(), "plugin.go") {
		t.Fatal(w.Code, w.Body.String())
	}
	if w = request(r, c, "POST", "/api/plugins/counter/toggle", `{"bot_id":"selected","enabled":false}`); !strings.Contains(w.Body.String(), `"code":0`) {
		t.Fatal(w.Body.String())
	}
}
