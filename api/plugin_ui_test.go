package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"shizukubot/config"
	"shizukubot/core"
	"shizukubot/plugins"
	"shizukubot/sdk"
	"strings"
	"testing"
	"time"
)

func addManagementBot(t *testing.T, r *Router, id string, allowed []string) *core.Bot {
	t.Helper()
	bot, err := core.NewBot(config.BotConfig{ID: id, Name: id, AppID: "mock", AppSecret: "mock", Mode: "webhook", Plugins: allowed})
	if err != nil {
		t.Fatal(err)
	}
	if err = r.botManager.AddBot(bot); err != nil {
		t.Fatal(err)
	}
	return bot
}
func envelope(t *testing.T, w *httptest.ResponseRecorder) Response {
	t.Helper()
	var response Response
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err, w.Body.String())
	}
	return response
}
func TestPluginManagementRequiresRealBotAndRespectsPolicy(t *testing.T) {
	r, c := setupRouter(t)
	for _, tc := range []struct{ method, path, body string }{
		{"GET", "/api/plugins?bot_id=missing", ""}, {"GET", "/api/plugins/hello?bot_id=missing", ""},
		{"PUT", "/api/plugins/hello", `{"bot_id":"missing","config":{}}`},
		{"POST", "/api/plugins/hello/toggle", `{"bot_id":"missing","enabled":true}`},
		{"POST", "/api/plugins/hello/restart", `{"bot_id":"missing"}`},
	} {
		if out := envelope(t, request(r, c, tc.method, tc.path, tc.body)); out.Code != 404 {
			t.Fatal(tc, out)
		}
	}
	addManagementBot(t, r, "a", []string{"hello"})
	addManagementBot(t, r, "b", nil)
	if out := envelope(t, request(r, c, "POST", "/api/plugins/counter/toggle", `{"bot_id":"a","enabled":true}`)); out.Code == 0 {
		t.Fatal("excluded plugin enabled")
	}
	if out := envelope(t, request(r, c, "POST", "/api/plugins/hello/toggle", `{"bot_id":"a","enabled":true}`)); out.Code != 0 {
		t.Fatal(out)
	}
	if r.plugins.Config("hello", "b").Enabled {
		t.Fatal("operation crossed bot scope")
	}
	out := request(r, c, "GET", "/api/plugins/hello?bot_id=a", "")
	if !strings.Contains(out.Body.String(), `"compiled":true`) || !strings.Contains(out.Body.String(), `"allowed":true`) || !strings.Contains(out.Body.String(), `"running":true`) || !strings.Contains(out.Body.String(), `"bot_running":false`) {
		t.Fatal(out.Body.String())
	}
	// Debugging an explicitly chosen compiled plugin remains available without production enable.
	out = request(r, c, "POST", "/api/bots/debug-id/sandbox/send", `{"content":"/ping","plugin_id":"hello"}`)
	var debug SandboxMessageResponse
	if err := json.Unmarshal(out.Body.Bytes(), &debug); err != nil || len(debug.Replies) != 1 {
		t.Fatal(out.Body.String())
	}
}
func TestConfigSaveReportsActualRestartAndDoesNotApplyInvalidConfig(t *testing.T) {
	r, c := setupRouter(t)
	addManagementBot(t, r, "a", nil)
	update := func(body string) map[string]any {
		t.Helper()
		response := envelope(t, request(r, c, "PUT", "/api/plugins/counter", body))
		if response.Code != 0 {
			t.Fatal(response)
		}
		return response.Data.(map[string]any)
	}
	out := update(`{"bot_id":"a","config":{"prefix":"one"}}`)
	if out["restart_required"] != false {
		t.Fatal(out)
	}
	if err := r.plugins.Enable(context.Background(), "counter", "a"); err != nil {
		t.Fatal(err)
	}
	out = update(`{"bot_id":"a","config":{"prefix":"one"}}`)
	if out["restart_required"] != false {
		t.Fatal(out)
	}
	out = update(`{"bot_id":"a","config":{"prefix":"two"}}`)
	if out["restart_required"] != true {
		t.Fatal(out)
	}
	response := envelope(t, request(r, c, "PUT", "/api/plugins/counter", `{"bot_id":"a","config":{"prefix":3},"apply":true}`))
	if response.Code == 0 || r.plugins.Config("counter", "a").Config["prefix"] != "two" {
		t.Fatal("invalid config changed saved settings", response)
	}
	out = update(`{"bot_id":"a","config":{"prefix":"two"},"apply":true}`)
	if out["applied"] != true || out["restart_required"] != false {
		t.Fatal(out)
	}
}

type failOnConfig struct{ sdk.Base }

func (*failOnConfig) Load(_ context.Context, h sdk.Host) error {
	if h.Config()["fail"] == true {
		return context.DeadlineExceeded
	}
	return h.Command("ok", func(context.Context, sdk.Call) error { return nil })
}
func TestConfigSavedAndRestartFailedAreDistinct(t *testing.T) {
	d := testDefinition("config-failure", func() plugins.Extension { return &failOnConfig{} })
	d.Manifest.Permissions = append(d.Manifest.Permissions, "config.read")
	d.Manifest.ConfigSchema = sdk.Schema{Type: "object", Properties: map[string]sdk.Schema{"fail": {Type: "boolean", Default: []byte(`false`)}}}
	r, c := setupRouterWith(t, d)
	addManagementBot(t, r, "a", nil)
	if err := r.plugins.Enable(context.Background(), d.Manifest.ID, "a"); err != nil {
		t.Fatal(err)
	}
	response := envelope(t, request(r, c, "PUT", "/api/plugins/config-failure", `{"bot_id":"a","config":{"fail":true},"apply":true}`))
	data := response.Data.(map[string]any)
	if response.Code != 0 || data["saved"] != true || data["applied"] != false || data["restart_error"] == nil || r.plugins.Config(d.Manifest.ID, "a").Config["fail"] != true {
		t.Fatal(response)
	}
}
func TestSandboxDistinguishesUnmatchedNoReplyAndTimeout(t *testing.T) {
	r, c := setupRouterWith(t, testDefinition("quiet", func() plugins.Extension {
		return &callbackExtension{command: "quiet", handle: func(context.Context, plugins.Call) error { return nil }}
	}), testDefinition("slow", func() plugins.Extension {
		return &callbackExtension{command: "slow", handle: func(ctx context.Context, _ plugins.Call) error { <-ctx.Done(); return ctx.Err() }}
	}))
	r.plugins.Timeout = 30 * time.Millisecond
	for _, tc := range []struct{ plugin, message, outcome string }{{"hello", "/unknown", "unmatched"}, {"hello", "/ping", "replied"}, {"quiet", "/quiet", "no_reply"}, {"slow", "/slow", "timeout"}} {
		w := request(r, c, "POST", "/api/bots/debug/sandbox/send", `{"plugin_id":"`+tc.plugin+`","content":"`+tc.message+`"}`)
		var out SandboxMessageResponse
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || len(out.Executions) != 1 {
			t.Fatal(w.Body.String())
		}
		if out.Executions[0].Outcome != tc.outcome || out.Executions[0].Matched == nil || out.Executions[0].DurationMS < 0 {
			t.Fatal(out)
		}
	}
}
func TestBackupRestoreExactSizeAndFailurePreservesData(t *testing.T) {
	r, c := setupRouter(t)
	addManagementBot(t, r, "a", nil)
	base := `{"version":1,"plugin_id":"hello","bots":{"a":{"config":{},"state":{}}}}`
	raw := base + strings.Repeat(" ", plugins.MaxBackupBytes-len(base))
	if out := envelope(t, request(r, c, "POST", "/api/plugins/data/restore", raw)); out.Code != 0 {
		t.Fatal(out)
	}
	before, _ := r.plugins.ExportBackup("hello")
	if w := request(r, c, "POST", "/api/plugins/data/restore", raw+" "); w.Code != http.StatusRequestEntityTooLarge {
		t.Fatal(w.Code, w.Body.String())
	}
	if out := envelope(t, request(r, c, "POST", "/api/plugins/data/restore", base)); out.Code == 0 {
		t.Fatal("existing data overwritten")
	}
	after, _ := r.plugins.ExportBackup("hello")
	if string(before) != string(after) {
		t.Fatal("failed restore damaged data")
	}
}
func TestGlobalConfigurationDoesNotDisableBots(t *testing.T) {
	r, c := setupRouter(t)
	bot := addManagementBot(t, r, "a", nil)
	bot.Enabled = true // Set before any concurrent work; do not start a real connection.
	out := envelope(t, request(r, c, "PUT", "/api/config", `{"webui":{"port":4545,"enabled":true},"bots":[{"id":"a","enabled":false}]}`))
	if out.Code != 0 || !bot.IsEnabled() {
		t.Fatal("global save disabled a bot", out)
	}
}
