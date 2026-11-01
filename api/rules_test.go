package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"shizukubot/events"
	"shizukubot/plugins"
	"shizukubot/rules"
	"shizukubot/webui"
	"strings"
	"testing"
)

func apiRule() rules.Rule {
	return rules.Rule{ID: "greeting", Name: "问候", Enabled: true, Priority: 100, Trigger: rules.Trigger{Type: "contains", Value: "你好"}, Conditions: rules.Conditions{Sources: []events.Source{events.Group, events.C2C}, GroupIDs: []string{}}, Action: rules.Action{Type: "reply_text", Text: "你好！"}, StopOnMatch: true}
}
func rulesBody(t *testing.T, revision uint64, rs ...rules.Rule) string {
	t.Helper()
	if rs == nil {
		rs = []rules.Rule{}
	}
	data, err := json.Marshal(map[string]any{"revision": revision, "rules": rs})
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
func TestRulesCRUDScopeConflictAndImmediateExecution(t *testing.T) {
	r, c := setupRouter(t)
	addManagementBot(t, r, "a", nil)
	addManagementBot(t, r, "b", nil)
	for _, method := range []string{"GET", "PUT", "POST"} {
		path := "/api/bots/missing/rules"
		body := rulesBody(t, 0, apiRule())
		if method == "POST" {
			path += "/test"
			body = `{"message":"你好","source":"group","group_id":"g"}`
		}
		if out := envelope(t, request(r, c, method, path, body)); out.Code != 404 {
			t.Fatal(out)
		}
	}
	if out := envelope(t, request(r, c, "PUT", "/api/bots/a/rules", rulesBody(t, 0, apiRule()))); out.Code != 0 {
		t.Fatal(out)
	}
	if out := envelope(t, request(r, c, "PUT", "/api/bots/a/rules", rulesBody(t, 0))); out.Code != 409 {
		t.Fatal(out)
	}
	other, _ := r.rules.Snapshot(context.Background(), "b")
	if other.Revision != 0 || len(other.Rules) != 0 {
		t.Fatal(other)
	}
	event := events.Event{BotID: "a", Source: events.C2C, Type: "c2c_message", Message: "你好", Reply: func(_ context.Context, reply events.Reply) error {
		if reply.Text != "你好！" {
			t.Error(reply)
		}
		return nil
	}}
	if out, err := r.rules.Execute(context.Background(), event); err != nil || len(out.Replies) != 1 {
		t.Fatal(out, err)
	}
	disabled := apiRule()
	disabled.Enabled = false
	if out := envelope(t, request(r, c, "PUT", "/api/bots/a/rules", rulesBody(t, 1, disabled))); out.Code != 0 {
		t.Fatal(out)
	}
	if out, err := r.rules.Execute(context.Background(), event); err != nil || len(out.Replies) != 0 {
		t.Fatal(out, err)
	}
	if out := envelope(t, request(r, c, "PUT", "/api/bots/a/rules", rulesBody(t, 2))); out.Code != 0 {
		t.Fatal(out)
	}
	if out := envelope(t, request(r, c, "GET", "/api/bots/a/rules", "")); out.Code != 0 || out.Data.(map[string]any)["revision"] != float64(3) {
		t.Fatal(out)
	}
}
func TestRulesRejectInvalidRequestsWithoutPartialApply(t *testing.T) {
	r, c := setupRouter(t)
	addManagementBot(t, r, "a", nil)
	bad := apiRule()
	bad.Trigger.Value = ""
	over := apiRule()
	over.Conditions.GroupIDs = make([]string, rules.MaxGroupIDs+1)
	for _, body := range []string{`{}`, `null`, `{"revision":0,"rules":null}`, `{"revision":0,"rules":[],"bot_id":"b"}`, `{"revision":0,"rules":[]} {}`, rulesBody(t, 0, bad), rulesBody(t, 0, over), rulesBody(t, 0, apiRule(), apiRule()), `{"revision":-1,"rules":[]}`} {
		out := envelope(t, request(r, c, "PUT", "/api/bots/a/rules", body))
		if out.Code != 400 {
			t.Fatal(body, out)
		}
	}
	if out := envelope(t, request(r, c, "GET", "/api/bots/bad%20id/rules", "")); out.Code != 400 {
		t.Fatal(out)
	}
	if w := request(r, c, "PUT", "/api/bots/a/rules", strings.Repeat(" ", rules.MaxRequestBytes+1)); w.Code != 413 {
		t.Fatal(w.Code, w.Body.String())
	}
	set, err := r.rules.Snapshot(context.Background(), "a")
	if err != nil || set.Revision != 0 || len(set.Rules) != 0 {
		t.Fatal(set, err)
	}
	for _, body := range []string{`{"message":"hi","source":"guild"}`, `{"message":"hi","source":"group"}`, `{"message":"hi","source":"c2c","group_id":"g"}`, `{"message":"hi","source":"c2c","user_id":"bad id"}`, `{"message":"","source":"c2c"}`} {
		if out := envelope(t, request(r, c, "POST", "/api/bots/a/rules/test", body)); out.Code != 400 {
			t.Fatal(body, out)
		}
	}
}
func TestRulesPreviewDoesNotPublishOrChangePluginData(t *testing.T) {
	r, c := setupRouter(t)
	addManagementBot(t, r, "a", nil)
	if out := envelope(t, request(r, c, "PUT", "/api/bots/a/rules", rulesBody(t, 0, apiRule()))); out.Code != 0 {
		t.Fatal(out)
	}
	if err := r.plugins.Enable(context.Background(), "counter", "a"); err != nil {
		t.Fatal(err)
	}
	before, err := r.plugins.ExportBackup("counter")
	if err != nil {
		t.Fatal(err)
	}
	published := false
	off := r.botManager.Events().Subscribe(events.Filter{}, func(context.Context, events.Event) error { published = true; return nil })
	defer off()
	draft := apiRule()
	draft.Action.Text = "草稿 {用户ID}/{群ID}/{BotID}"
	raw, _ := json.Marshal(map[string]any{"message": "你好", "source": "group", "group_id": "g", "user_id": "u", "rules": []rules.Rule{draft}})
	w := request(r, c, "POST", "/api/bots/a/rules/test", string(raw))
	out := envelope(t, w)
	if out.Code != 0 {
		t.Fatal(out)
	}
	var response struct {
		Data rules.Result `json:"data"`
	}
	if err = json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !response.Data.Draft || len(response.Data.Replies) != 1 || response.Data.Replies[0] != "草稿 u/g/a" || published {
		t.Fatal(response, published)
	}
	after, err := r.plugins.ExportBackup("counter")
	if err != nil || string(before) != string(after) {
		t.Fatal("preview changed plugin state", err)
	}
	set, err := r.rules.Snapshot(context.Background(), "a")
	if err != nil || set.Revision != 1 || set.Rules[0].Action.Text != "你好！" {
		t.Fatal(set, err)
	}
}
func TestRulesBuiltinPluginOrderAndFailureIsolation(t *testing.T) {
	r, _ := setupRouterWith(t, testDefinition("listener", func() plugins.Extension {
		return &callbackExtension{event: "*", handle: func(ctx context.Context, c plugins.Call) error { return c.Reply(ctx, "plugin") }}
	}))
	addManagementBot(t, r, "a", nil)
	if err := r.plugins.Enable(context.Background(), "listener", "a"); err != nil {
		t.Fatal(err)
	}
	rule := apiRule()
	rule.Trigger.Value = "/"
	rule.Action.Text = "rule"
	if _, err := r.rules.Replace(context.Background(), "a", 0, []rules.Rule{rule}); err != nil {
		t.Fatal(err)
	}
	for _, msg := range []string{"/help", "/status", "#shizuku", "/ping"} {
		var replies []string
		event := events.Event{BotID: "a", Source: events.C2C, Type: "c2c_message", Message: msg, Reply: func(_ context.Context, r events.Reply) error { replies = append(replies, r.Text); return nil }}
		if err := r.botManager.Events().Publish(context.Background(), event); err != nil {
			t.Fatal(err)
		}
		if msg == "/ping" {
			if len(replies) != 2 || replies[0] != "rule" || replies[1] != "plugin" {
				t.Fatal(replies)
			}
		} else {
			if len(replies) != 1 || replies[0] == "rule" || replies[0] == "plugin" {
				t.Fatal(msg, replies)
			}
			out, err := r.rules.Preview(context.Background(), event, nil)
			if err != nil || out.SkipReason != "builtin_command" || len(out.Replies) != 0 {
				t.Fatal(out, err)
			}
		}
	}
	var replies []string
	err := r.botManager.Events().Publish(context.Background(), events.Event{BotID: "a", Source: events.C2C, Type: "c2c_message", Message: "/ping", Reply: func(_ context.Context, r events.Reply) error {
		if r.Text == "rule" {
			return errors.New("mock transport failure")
		}
		replies = append(replies, r.Text)
		return nil
	}})
	if err == nil || len(replies) != 1 || replies[0] != "plugin" {
		t.Fatal(replies, err)
	}
}
func TestRulesAdminCSRFAndWriteFailure(t *testing.T) {
	r, c := setupRouter(t)
	addManagementBot(t, r, "a", nil)
	if err := webui.GetUserStore().CreateUser("viewer", "viewer-password", false); err != nil {
		t.Fatal(err)
	}
	viewer := request(r, nil, "POST", "/login", `{"username":"viewer","password":"viewer-password"}`).Result().Cookies()[0]
	for _, method := range []string{"GET", "PUT", "POST"} {
		path := "/api/bots/a/rules"
		if method == "POST" {
			path += "/test"
		}
		if w := request(r, nil, method, path, "{}"); w.Code != 401 {
			t.Fatal(w.Code)
		}
		if w := request(r, viewer, method, path, "{}"); w.Code != 403 {
			t.Fatal(w.Code)
		}
		if method != "GET" {
			req := httptest.NewRequest(method, path, strings.NewReader("{}"))
			req.AddCookie(c)
			req.Header.Set("Origin", "https://evil.invalid")
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.WithCORS().ServeHTTP(w, req)
			if w.Code != 403 {
				t.Fatal(w.Code)
			}
		}
	}
	if out := envelope(t, request(r, c, "PUT", "/api/bots/a/rules", rulesBody(t, 0, apiRule()))); out.Code != 0 {
		t.Fatal(out)
	}
	paths, err := filepath.Glob("data/rules/*.json")
	if err != nil || len(paths) != 1 {
		t.Fatal(paths, err)
	}
	if err = os.Rename(paths[0], paths[0]+".preserved"); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(paths[0], 0700); err != nil {
		t.Fatal(err)
	}
	out := envelope(t, request(r, c, "PUT", "/api/bots/a/rules", rulesBody(t, 1)))
	if out.Code != 500 || strings.Contains(out.Message, "data/rules") {
		t.Fatal(out)
	}
	set, err := r.rules.Snapshot(context.Background(), "a")
	if err != nil || set.Revision != 1 || len(set.Rules) != 1 {
		t.Fatal(set, err)
	}
}
