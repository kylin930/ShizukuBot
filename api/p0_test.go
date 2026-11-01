package api

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/gorilla/websocket"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"shizukubot/config"
	"shizukubot/core"
	"shizukubot/plugins"
	"shizukubot/webui"
	"strings"
	"testing"
	"time"
)

func fixturePackage(t *testing.T, id, code string) []byte {
	t.Helper()
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	mf := plugins.Manifest{APIVersion: 1, ID: id, Name: id, Version: "1.0.0", Runtime: "javascript", Entry: "dist/index.js", Permissions: []string{"commands.register", "message.send"}, ConfigSchema: plugins.Schema{Type: "object"}}
	raw, _ := json.Marshal(mf)
	for name, b := range map[string][]byte{"manifest.json": raw, "dist/index.js": []byte(code)} {
		w, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write(b)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func setupRouter(t *testing.T) (*Router, *http.Cookie) {
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
	r := NewRouter(bm, cfg, "config/config.json", bm.GetLogManager())
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
func TestSandboxLegacySelectedBotAndNoLiveSending(t *testing.T) {
	r, c := setupRouter(t)
	// No real Bot/OpenAPI exists. All sources must still execute and capture replies.
	p, err := r.plugins.Store.Install(fixturePackage(t, "fixture", `export default {id:'fixture',version:'1.0.0',commands:{ping:async c=>c.reply('pong')}};`))
	if err != nil {
		t.Fatal(err)
	}
	if err = r.plugins.Trust("fixture", "selected", true); err != nil {
		t.Fatal(err)
	}

	for _, typ := range []string{"group", "c2c"} {
		w := request(r, c, "POST", "/api/bots/selected/sandbox/send", `{"type":"`+typ+`","content":"/ping","plugin_id":"fixture"}`)
		var result SandboxMessageResponse
		if json.Unmarshal(w.Body.Bytes(), &result) != nil || len(result.Replies) != 1 || result.Replies[0] != "pong" || len(result.Errors) != 0 || len(result.Executions) != 1 || result.Executions[0].Status != "completed" {
			t.Fatal(w.Body.String())
		}
	}
	if err := os.WriteFile(filepath.Join(p.Directory, "dist/index.js"), []byte(`export default {id:'fixture',version:'1.0.0',commands:{ping:()=>{throw Error('fixture failure')}}};`), 0600); err != nil {
		t.Fatal(err)
	}

	w := request(r, c, "POST", "/api/bots/selected/sandbox/send", `{"content":"/ping","plugin_id":"fixture"}`)
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
	for _, route := range []string{"/api/code/scan", "/api/code/code2graph", "/api/code/graph2code", "/api/plugins/test/graph", "/api/custom-nodes"} {
		w := request(r, c, "POST", route, `{"path":"/etc/passwd"}`)
		if w.Code < 400 {
			t.Fatal("retired execution route still callable", route, w.Code)
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
	r, c := setupRouter(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	mf := plugins.Manifest{APIVersion: 1, ID: "blocking", Name: "blocking", Version: "1.0.0", Runtime: "go", Permissions: []string{"commands.register"}, ConfigSchema: plugins.Schema{Type: "object"}}
	if err := r.plugins.RegisterGo(mf, func() plugins.Extension { return &blockingExtension{entered: entered, release: release} }); err != nil {
		t.Fatal(err)
	}
	if err := r.plugins.Trust("blocking", "chosen", true); err != nil {
		t.Fatal(err)
	}
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
	r, c := setupRouter(t)
	code := `export default {id:'media',version:'1.0.0',commands:{ping:async c=>{await c.reply('text');await c.replyImage('https://example.com/a');await c.replyRichMedia('text','https://example.com/a');await c.replyMarkdown('md');await c.replyKeyboard('md','{}');await c.replyVideo('https://example.com/v');}}};`
	if _, err := r.plugins.Store.Install(fixturePackage(t, "media", code)); err != nil {
		t.Fatal(err)
	}
	r.plugins.Trust("media", "chosen", true)
	w := request(r, c, "POST", "/api/bots/chosen/sandbox/send", `{"content":"/ping","plugin_id":"media"}`)
	var result SandboxMessageResponse
	if json.Unmarshal(w.Body.Bytes(), &result) != nil || len(result.Results) != 6 || len(result.Errors) != 0 {
		t.Fatal(w.Body.String())
	}
}
