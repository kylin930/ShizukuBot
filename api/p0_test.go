package api

import (
	"bytes"
	"encoding/json"
	"github.com/gorilla/websocket"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"shizukubot/config"
	"shizukubot/core"
	"shizukubot/plugin"
	"shizukubot/webui"
	"strings"
	"testing"
	"time"
)

const sandboxGraph = `{"nodes":[{"id":1,"type":"event/on_command","properties":{"command":"ping","prefix":"/"},"outputs":[{"name":"触发","type":-1,"links":[1]}]},{"id":2,"type":"action/send_message","properties":{"template":"pong"},"inputs":[{"name":"触发","type":-1,"link":1}]}],"links":[[1,1,0,2,0,-1]]}`

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
	p := &plugin.Plugin{ID: "fixture", Name: "fixture", Enabled: false, BotID: "selected", Graph: json.RawMessage(sandboxGraph)}
	if err := r.pluginStore.Save(p); err != nil {
		t.Fatal(err)
	}
	for _, typ := range []string{"group", "c2c"} {
		w := request(r, c, "POST", "/api/bots/selected/sandbox/send", `{"type":"`+typ+`","content":"/ping","plugin_id":"fixture"}`)
		var result SandboxMessageResponse
		if json.Unmarshal(w.Body.Bytes(), &result) != nil || len(result.Replies) != 1 || result.Replies[0] != "pong" || len(result.Errors) != 0 || len(result.Executions) != 1 || result.Executions[0].Status != "completed" {
			t.Fatal(w.Body.String())
		}
	}
	p.Graph = json.RawMessage(strings.Replace(sandboxGraph, "action/send_message", "missing/action", 1))
	r.pluginStore.Save(p)
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
	outside := filepath.Join(t.TempDir(), "secret.go")
	os.WriteFile(outside, []byte("package private\n"), 0600)
	os.Symlink(outside, "data/escape.go")
	w := request(r, c, "POST", "/api/code/scan", `{"source_type":"file","path":"data/escape.go"}`)
	if strings.Contains(w.Body.String(), `"code":0`) {
		t.Fatal("scan symlink escaped", w.Body.String())
	}
	os.WriteFile("data/valid.go", []byte("package demo\nfunc Example() {}"), 0600)
	w = request(r, c, "POST", "/api/code/scan", `{"source_type":"file","path":"data/valid.go"}`)
	if !strings.Contains(w.Body.String(), `"code":0`) {
		t.Fatal("valid scan failed", w.Body.String())
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
