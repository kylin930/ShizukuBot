package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kylin930/botgo-plus"
	"github.com/kylin930/botgo-plus/errs"
	"github.com/kylin930/botgo-plus/openapi"
	"shizukubot/events"
	"shizukubot/plugins"
	"shizukubot/plugins/hello"
	"shizukubot/rules"
)

type qqRequest struct {
	Method, Path, App string
	Body              map[string]any
}
type qqMock struct {
	mu       sync.Mutex
	requests []qqRequest
	server   *httptest.Server
}

func newQQMock(t *testing.T) *qqMock {
	t.Helper()
	m := &qqMock{}
	m.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		m.mu.Lock()
		m.requests = append(m.requests, qqRequest{r.Method, r.URL.Path, r.Header.Get("X-Union-Appid"), body})
		m.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/files") {
			fmt.Fprint(w, `{"file_info":"b3BhcXVlLWZpbGU=","ttl":300}`)
		} else {
			fmt.Fprint(w, `{"id":"outgoing","timestamp":123}`)
		}
	}))
	t.Cleanup(m.server.Close)
	return m
}
func (m *qqMock) captured() []qqRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]qqRequest(nil), m.requests...)
}
func useQQEndpoint(t *testing.T, b *Bot, url string) {
	t.Helper()
	b.OpenAPI = botgo.NewOpenAPI(b.AppID, b.TokenSource)
	b.OpenAPI = b.OpenAPI.(interface{ WithBaseURL(string) openapi.OpenAPI }).WithBaseURL(url)
}
func incoming(id, kind, content string) []byte {
	return []byte(fmt.Sprintf(`{"op":0,"id":"event-%s","t":%q,"d":{"id":%q,"content":%q,"group_openid":"group","author":{"user_openid":"user","member_openid":"member","id":"wrong-namespace"}}}`, id, kind, id, content))
}
func assertQQSend(t *testing.T, r qqRequest, path, id, text string, seq float64, kind float64) {
	t.Helper()
	if r.Method != "POST" || r.Path != path || r.Body["msg_id"] != id || r.Body["msg_seq"] != seq || r.Body["msg_type"] != kind || (text != "" && r.Body["content"] != text) {
		t.Fatalf("incorrect QQ HTTP request: %+v", r)
	}
	if _, ok := r.Body["event_id"]; ok {
		t.Fatalf("message reply used event_id: %+v", r)
	}
}
func TestQQProtocolPluginsRulesBuiltinsAndBotIsolation(t *testing.T) {
	mock := newQQMock(t)
	bm := NewBotManager()
	t.Cleanup(bm.StopAll)
	registry := plugins.NewRegistry()
	if err := registry.Register(hello.Definition()); err != nil {
		t.Fatal(err)
	}
	pm, err := plugins.NewManager(registry, bm.Events(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pm.Close(context.Background()) })
	store, err := rules.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	engine := rules.NewEngine(store)
	t.Cleanup(func() { engine.Close(context.Background()) })
	if err = engine.Attach(bm.Events()); err != nil {
		t.Fatal(err)
	}
	rule := rules.Rule{ID: "reply", Name: "reply", Enabled: true, Trigger: rules.Trigger{Type: "contains", Value: "/ping"}, Conditions: rules.Conditions{Sources: []events.Source{events.C2C, events.Group}}, Action: rules.Action{Type: "reply_text", Text: "rule reply"}}
	for _, id := range []string{"a", "b"} {
		b := testBot(t, id)
		b.AppID = "app-" + id
		useQQEndpoint(t, b, mock.server.URL)
		if err = bm.AddBot(b); err != nil {
			t.Fatal(err)
		}
		if err = b.Start(); err != nil {
			t.Fatal(err)
		}
		if err = pm.Enable(context.Background(), "hello", id); err != nil {
			t.Fatal(err)
		}
		if _, err = engine.Replace(context.Background(), id, 0, []rules.Rule{rule}); err != nil {
			t.Fatal(err)
		}
		// Rule subscribed first, plugin second: same event closure, unique seqs.
		for _, kind := range []string{"C2C_MESSAGE_CREATE", "GROUP_AT_MESSAGE_CREATE", "GROUP_MESSAGE_CREATE"} {
			raw := incoming(kind, kind, "/ping")
			if err = b.HandlePayload(context.Background(), raw); err != nil {
				t.Fatal(err)
			}
			if err = b.HandlePayload(context.Background(), raw); err != nil {
				t.Fatal(err)
			}
		}
	}
	reqs := mock.captured()
	if len(reqs) != 12 {
		t.Fatalf("dedup/count: %d", len(reqs))
	}
	for i, r := range reqs {
		kinds := []string{"C2C_MESSAGE_CREATE", "GROUP_AT_MESSAGE_CREATE", "GROUP_MESSAGE_CREATE"}
		kind := kinds[(i%6)/2]
		path := "/v2/groups/group/messages"
		if i%6 < 2 {
			path = "/v2/users/user/messages"
		}
		content := "rule reply"
		if i%2 == 1 {
			content = "pong"
		}
		assertQQSend(t, r, path, kind, content, float64(i%2+1), 0)
		app := "app-a"
		if i >= 6 {
			app = "app-b"
		}
		if r.App != app {
			t.Fatalf("bot crossed: %s", r.App)
		}
	}
	// Saved rule edits take effect without reconnect and coexist with the plugin.
	rule.Action.Text = "changed"
	if _, err = engine.Replace(context.Background(), "a", 1, []rules.Rule{rule}); err != nil {
		t.Fatal(err)
	}
	a, _ := bm.GetBot("a")
	if err = a.HandlePayload(context.Background(), incoming("new", "C2C_MESSAGE_CREATE", "/ping")); err != nil {
		t.Fatal(err)
	}
	reqs = mock.captured()
	assertQQSend(t, reqs[12], "/v2/users/user/messages", "new", "changed", 1, 0)
	bm.Events().Subscribe(events.Filter{BotID: "a"}, func(ctx context.Context, e events.Event) error { a.HandleBuiltinCommand(ctx, e); return nil })
	if err = a.HandlePayload(context.Background(), incoming("builtin", "C2C_MESSAGE_CREATE", "#shizuku")); err != nil {
		t.Fatal(err)
	}
	reqs = mock.captured()
	assertQQSend(t, reqs[14], "/v2/users/user/messages", "builtin", "", 1, 0)
}
func TestQQProtocolConcurrentSequencesLimitsAndRelease(t *testing.T) {
	mock := newQQMock(t)
	b := testBot(t, "seq")
	useQQEndpoint(t, b, mock.server.URL)
	e := events.Event{Type: "group_at_message", PlatformType: "GROUP_AT_MESSAGE_CREATE", Source: events.Group, GroupID: "group", MessageID: "original"}
	ctx, cancel := context.WithCancel(context.Background())
	reply := b.replyContext(ctx, e)
	var wg sync.WaitGroup
	var successes, limited atomic.Int32
	for range 20 {
		wg.Go(func() {
			err := reply(context.Background(), events.Reply{Kind: "text", Text: "reply"})
			if err == nil {
				successes.Add(1)
			} else if errors.Is(err, ErrReplyLimit) {
				limited.Add(1)
			} else {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if successes.Load() != 5 || limited.Load() != 15 {
		t.Fatal(successes.Load(), limited.Load())
	}
	seen := map[any]bool{}
	for _, r := range mock.captured() {
		seq := r.Body["msg_seq"]
		if seen[seq] {
			t.Fatal("duplicate seq")
		}
		seen[seq] = true
	}
	cancel()
	if !errors.Is(reply(context.Background(), events.Reply{Kind: "text"}), context.Canceled) {
		t.Fatal("retained closure survived event end")
	}
	// New event naturally owns a fresh counter; same ID across Bots never shares it.
	if err := b.reply(e)(context.Background(), events.Reply{Kind: "text", Text: "new"}); err != nil {
		t.Fatal(err)
	}
	reqs := mock.captured()
	assertQQSend(t, reqs[5], "/v2/groups/group/messages", "original", "new", 1, 0)
}
func TestQQProtocolAllMediaFormats(t *testing.T) {
	for _, source := range []events.Source{events.Group, events.C2C} {
		t.Run(string(source), func(t *testing.T) {
			mock := newQQMock(t)
			b := testBot(t, "media")
			useQQEndpoint(t, b, mock.server.URL)
			for _, kind := range []string{"text", "markdown", "keyboard", "image", "rich_media", "video", "base64"} {
				e := events.Event{Type: "group_at_message", Source: source, GroupID: "group", UserID: "user", MessageID: kind}
				r := events.Reply{Kind: kind, Text: "body", URL: "https://example.invalid/file", Keyboard: `{"id":"approved-template"}`}
				if kind == "base64" {
					r.Kind = "image"
					r.URL = "data:image/png;base64,aW1hZ2U="
				}
				before := len(mock.captured())
				if err := b.reply(e)(context.Background(), r); err != nil {
					t.Fatal(err)
				}
				requests := mock.captured()[before:]
				path := "/v2/groups/group"
				if source == events.C2C {
					path = "/v2/users/user"
				}
				msgType := float64(0)
				switch kind {
				case "markdown", "keyboard":
					msgType = 2
				case "image", "rich_media", "video", "base64":
					msgType = 7
					if len(requests) != 2 {
						t.Fatal(requests)
					}
					upload := requests[0]
					if upload.Method != "POST" || upload.Path != path+"/files" || upload.Body["srv_send_msg"] != false {
						t.Fatal(upload)
					}
					fileType := float64(1)
					if kind == "video" {
						fileType = 2
					}
					if upload.Body["file_type"] != fileType {
						t.Fatal(upload)
					}
					if kind == "base64" && upload.Body["file_data"] != "aW1hZ2U=" {
						t.Fatal(upload)
					}
				}
				final := requests[len(requests)-1]
				assertQQSend(t, final, path+"/messages", kind, "", 1, msgType)
				if msgType == 7 && final.Body["media"].(map[string]any)["file_info"] != "b3BhcXVlLWZpbGU=" {
					t.Fatal(final)
				}
				if msgType == 2 && final.Body["markdown"].(map[string]any)["content"] != "body" {
					t.Fatal(final)
				}
				if kind == "keyboard" && final.Body["keyboard"].(map[string]any)["id"] != "approved-template" {
					t.Fatal(final)
				}
			}
		})
	}
}
func TestQQProtocolMissingIDSupportedEventIDsAndActiveSend(t *testing.T) {
	mock := newQQMock(t)
	b := testBot(t, "ids")
	useQQEndpoint(t, b, mock.server.URL)
	msg := events.Reply{Kind: "text", Text: "reply"}
	for _, e := range []events.Event{{Type: "c2c_message", Source: events.C2C, UserID: "user", EventID: "not-a-message"}, {Type: "interaction", PlatformType: "INTERACTION_CREATE", EventID: "interaction"}} {
		if err := b.reply(e)(context.Background(), msg); err == nil {
			t.Fatal("unsupported/missing ID accepted")
		}
	}
	if len(mock.captured()) != 0 {
		t.Fatal("fabricated reply")
	}
	for _, kind := range []string{"FRIEND_ADD", "C2C_MSG_RECEIVE", "GROUP_ADD_ROBOT", "GROUP_MSG_RECEIVE"} {
		source := events.C2C
		if strings.HasPrefix(kind, "GROUP") {
			source = events.Group
		}
		e := events.Event{Source: source, PlatformType: kind, EventID: "event", UserID: "user", GroupID: "group"}
		if err := b.reply(e)(context.Background(), msg); err != nil {
			t.Fatal(err)
		}
	}
	for _, r := range mock.captured() {
		if r.Body["event_id"] != "event" || r.Body["msg_id"] != nil || r.Body["msg_seq"] != float64(1) {
			t.Fatal(r)
		}
	}
}
func TestQQProtocolPlatformErrorsTimeoutAndNoRetry(t *testing.T) {
	for _, tc := range []struct {
		name         string
		status, code int
		category     string
	}{{"auth", 401, 11244, "authentication"}, {"rate", 429, 22009, "platform_limit"}, {"unknown", 400, 999123, "unknown_platform_error"}, {"business200", 200, 999123, "unknown_platform_error"}, {"forbidden", 403, 0, "permission_or_ip_policy"}} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("X-Tps-Trace-Id", "trace")
				w.WriteHeader(tc.status)
				fmt.Fprintf(w, `{"code":%d,"message":"private-token-must-not-leak"}`, tc.code)
			}))
			defer server.Close()
			b := testBot(t, "error")
			useQQEndpoint(t, b, server.URL)
			err := b.reply(events.Event{Type: "c2c_message", Source: events.C2C, MessageID: "id", UserID: "user"})(context.Background(), events.Reply{Kind: "text", Text: "private"})
			var p *errs.PlatformError
			if !errors.As(err, &p) || p.Status != tc.status || p.Code != tc.code || strings.Contains(err.Error(), "private") || calls.Load() != 1 {
				t.Fatalf("%v calls=%d", err, calls.Load())
			}
			if b.Diagnostics().LastSendFailure != tc.category {
				t.Fatal(b.Diagnostics())
			}
		})
	}
	t.Run("timeout-consumes-sequence", func(t *testing.T) {
		var calls atomic.Int32
		var mu sync.Mutex
		var seqs []float64
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			mu.Lock()
			seqs = append(seqs, body["msg_seq"].(float64))
			mu.Unlock()
			time.Sleep(60 * time.Millisecond)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"id":"sent"}`)
		}))
		defer server.Close()
		b := testBot(t, "timeout")
		useQQEndpoint(t, b, server.URL)
		b.OpenAPI.WithTimeout(20 * time.Millisecond)
		reply := b.reply(events.Event{Type: "c2c_message", Source: events.C2C, MessageID: "id", UserID: "user"})
		for range 2 {
			if err := reply(context.Background(), events.Reply{Kind: "text", Text: "reply"}); !errors.Is(err, context.DeadlineExceeded) {
				t.Fatal(err)
			}
		}
		if calls.Load() != 2 {
			t.Fatal("automatic retry", calls.Load())
		}
		mu.Lock()
		defer mu.Unlock()
		if len(seqs) != 2 || seqs[0] != 1 || seqs[1] != 2 {
			t.Fatal(seqs)
		}
	})
}

func TestQQProtocolHTTPCancellationAndDisconnect(t *testing.T) {
	for _, disconnect := range []bool{false, true} {
		t.Run(fmt.Sprint(disconnect), func(t *testing.T) {
			entered := make(chan struct{})
			var count atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				count.Add(1)
				// Consume body before waiting for the transport to signal cancellation.
				var body map[string]any
				json.NewDecoder(r.Body).Decode(&body)
				close(entered)
				if disconnect {
					conn, _, err := w.(http.Hijacker).Hijack()
					if err == nil {
						conn.Close()
					}
					return
				}
				select {
				case <-r.Context().Done():
				case <-time.After(time.Second):
				}
			}))
			defer server.Close()
			b := testBot(t, "cancel-http")
			useQQEndpoint(t, b, server.URL)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() {
				done <- b.reply(events.Event{Type: "c2c_message", Source: events.C2C, UserID: "user", MessageID: "id"})(ctx, events.Reply{Kind: "text", Text: "reply"})
			}()
			<-entered
			if !disconnect {
				cancel()
			}
			select {
			case err := <-done:
				if err == nil || (!disconnect && !errors.Is(err, context.Canceled)) {
					t.Fatal(err)
				}
			case <-time.After(time.Second):
				t.Fatal("HTTP send did not finish")
			}
			if count.Load() != 1 {
				t.Fatal("retried uncertain delivery", count.Load())
			}
		})
	}
}

func TestQQProtocolC2CLimitAndExplicitActiveSend(t *testing.T) {
	mock := newQQMock(t)
	b := testBot(t, "active")
	useQQEndpoint(t, b, mock.server.URL)
	if err := b.Start(); err != nil {
		t.Fatal(err)
	}
	defer b.Stop()
	reply := b.reply(events.Event{Type: "c2c_message", Source: events.C2C, UserID: "user", MessageID: "id"})
	for range 4 {
		if err := reply(context.Background(), events.Reply{Kind: "text", Text: "passive"}); err != nil {
			t.Fatal(err)
		}
	}
	if !errors.Is(reply(context.Background(), events.Reply{Kind: "text"}), ErrReplyLimit) {
		t.Fatal("C2C exceeded current official 4-reply limit")
	}
	for _, send := range []func() error{func() error { return b.SendC2CMessage("user", "explicit active") }, func() error { return b.SendGroupImageData("group", []byte("image")) }} {
		if err := send(); err != nil {
			t.Fatal(err)
		}
	}
	reqs := mock.captured()
	if len(reqs) != 7 {
		t.Fatal(len(reqs))
	}
	for _, r := range reqs[4:] {
		if r.Body["msg_id"] != nil || r.Body["event_id"] != nil || r.Body["msg_seq"] != nil {
			t.Fatal("active borrowed passive identity", r)
		}
	}
}

func TestQQProtocolOpaqueFileInfoAndMalformedSuccess(t *testing.T) {
	for _, tc := range []struct {
		upload, send string
		wantError    bool
	}{{`{"file_info":"opaque+/not-base64!"}`, `{"id":"ok","timestamp":"2026-07-21T10:30:00+08:00"}`, false}, {`{}`, `{"id":"ok"}`, true}, {`{"file_info":"opaque+/not-base64!"}`, `{}`, true}} {
		var sends atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if strings.HasSuffix(r.URL.Path, "/files") {
				fmt.Fprint(w, tc.upload)
				return
			}
			sends.Add(1)
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			if body["media"].(map[string]any)["file_info"] != "opaque+/not-base64!" {
				t.Error("file_info changed")
			}
			fmt.Fprint(w, tc.send)
		}))
		b := testBot(t, "opaque")
		useQQEndpoint(t, b, server.URL)
		err := b.reply(events.Event{Type: "c2c_message", Source: events.C2C, UserID: "user", MessageID: "id"})(context.Background(), events.Reply{Kind: "image", URL: "https://example.invalid/image"})
		server.Close()
		if (err != nil) != tc.wantError {
			t.Fatal(err)
		}
		if tc.upload == `{}` && sends.Load() != 0 {
			t.Fatal("sent missing file_info")
		}
	}
}
