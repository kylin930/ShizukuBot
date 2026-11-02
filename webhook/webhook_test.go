package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/kylin930/botgo-plus"
	"github.com/kylin930/botgo-plus/interaction/signature"
	"github.com/kylin930/botgo-plus/openapi"
	"golang.org/x/oauth2"
	"io"
	"net/http"
	"net/http/httptest"
	"shizukubot/config"
	"shizukubot/core"
	"shizukubot/events"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestSignedWebhookPerBotAndChunkedBody(t *testing.T) {
	bm := core.NewBotManager()
	defer bm.StopAll()
	for _, id := range []string{"a", "b"} {
		bot, err := core.NewBot(config.BotConfig{ID: id, AppID: id, AppSecret: "test-secret-" + id, Mode: "webhook", Enabled: true})
		if err != nil {
			t.Fatal(err)
		}
		bot.TokenSource = oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "mock", TokenType: "QQBot"})
		bm.AddBot(bot)
		if err = bot.Start(); err != nil {
			t.Fatal(err)
		}
	}
	counts := map[string]int{}
	bm.Events().Subscribe(events.Filter{}, func(_ context.Context, e events.Event) error { counts[e.BotID]++; return nil })
	routes := mux.NewRouter()
	routes.Handle("/webhook/{botID}", NewHandler(bm))
	body := `{"op":0,"id":"event","t":"GROUP_AT_MESSAGE_CREATE","d":{"id":"message","group_openid":"g","content":"same"}}`
	for _, id := range []string{"a", "b"} {
		for range 2 {
			req := httptest.NewRequest("POST", "/webhook/"+id, strings.NewReader(body))
			req.ContentLength = -1
			req.Header.Set(signature.HeaderTimestamp, fmt.Sprint(time.Now().Unix()))
			sig, err := signature.Generate("test-secret-"+id, req.Header, []byte(body))
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set(signature.HeaderSig, sig)
			w := httptest.NewRecorder()
			routes.ServeHTTP(w, req)
			if w.Code != 200 || !strings.Contains(w.Body.String(), `"d":0`) {
				t.Fatal(w.Code, w.Body.String())
			}
		}
	}
	if counts["a"] != 1 || counts["b"] != 1 {
		t.Fatal(counts)
	}
	req := httptest.NewRequest(http.MethodPost, "/webhook/a", strings.NewReader(body))
	w := httptest.NewRecorder()
	routes.ServeHTTP(w, req)
	if w.Code != 401 {
		t.Fatal("unsigned callback accepted")
	}
}

func TestWebhookValidationErrorsAndAdmissionACK(t *testing.T) {
	bm := core.NewBotManager()
	defer bm.StopAll()
	bot, err := core.NewBot(config.BotConfig{ID: "a", AppID: "test", AppSecret: "secret", Mode: "webhook"})
	if err != nil {
		t.Fatal(err)
	}
	bot.TokenSource = oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "mock", TokenType: "QQBot"})
	if err = bm.AddBot(bot); err != nil {
		t.Fatal(err)
	}
	if err = bot.Start(); err != nil {
		t.Fatal(err)
	}
	routes := mux.NewRouter()
	routes.Handle("/webhook/{botID}", NewHandler(bm))
	request := func(path, body string, mutate func(*http.Request)) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", path, strings.NewReader(body))
		req.Header.Set(signature.HeaderTimestamp, fmt.Sprint(time.Now().Unix()))
		sig, err := signature.Generate("secret", req.Header, []byte(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set(signature.HeaderSig, sig)
		if mutate != nil {
			mutate(req)
		}
		w := httptest.NewRecorder()
		routes.ServeHTTP(w, req)
		return w
	}
	body := `{"op":0,"t":"C2C_MESSAGE_CREATE","d":{"id":"message","author":{"user_openid":"u"}}}`
	bm.Events().Subscribe(events.Filter{}, func(context.Context, events.Event) error { return fmt.Errorf("plugin failed") })
	w := request("/webhook/a", body, nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"d":0`) {
		t.Fatal("business failure requested replay", w.Code, w.Body.String())
	}
	for _, tc := range []struct {
		name, path, body string
		mutate           func(*http.Request)
		want             int
	}{
		{"unknown bot", "/webhook/missing", body, nil, 404},
		{"tampered body", "/webhook/a", body, func(r *http.Request) { r.Body = io.NopCloser(strings.NewReader(body + " ")) }, 401},
		{"wrong secret", "/webhook/a", body, func(r *http.Request) {
			sig, _ := signature.Generate("other", r.Header, []byte(body))
			r.Header.Set(signature.HeaderSig, sig)
		}, 401},
		{"missing timestamp", "/webhook/a", body, func(r *http.Request) { r.Header.Del(signature.HeaderTimestamp) }, 401},
		{"malformed JSON", "/webhook/a", "{", nil, 400},
		{"invalid data", "/webhook/a", `{"op":0,"t":"C2C_MESSAGE_CREATE","d":null}`, nil, 400},
		{"unsupported opcode", "/webhook/a", `{"op":99}`, nil, 400},
		{"bad challenge", "/webhook/a", `{"op":13,"d":{}}`, nil, 400},
		{"oversized", "/webhook/a", strings.Repeat("x", 2<<20+1), nil, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := request(tc.path, tc.body, tc.mutate)
			if w.Code != tc.want {
				t.Fatal(w.Code, w.Body.String())
			}
		})
	}
	challenge := `{"op":13,"d":{"plain_token":"challenge","event_ts":"12345"}}`
	w = request("/webhook/a", challenge, nil)
	var result struct {
		PlainToken string `json:"plain_token"`
		Signature  string `json:"signature"`
	}
	json.Unmarshal(w.Body.Bytes(), &result)
	header := http.Header{}
	header.Set(signature.HeaderTimestamp, "12345")
	header.Set(signature.HeaderSig, result.Signature)
	pass, err := signature.Verify("secret", header, []byte(result.PlainToken))
	if w.Code != 200 || result.PlainToken != "challenge" || err != nil || !pass {
		t.Fatal("invalid challenge response", w.Body.String(), err)
	}
	w = request("/webhook/a", `{"op":1,"d":42}`, nil)
	if !strings.Contains(w.Body.String(), `"op":11,"d":42`) {
		t.Fatal(w.Body.String())
	}
	bot.Stop()
	w = request("/webhook/a", body, nil)
	if !strings.Contains(w.Body.String(), `"d":1`) {
		t.Fatal("stopped bot acknowledged delivery", w.Body.String())
	}
}

func TestQQSignedCallbackToRealSDKHTTP(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if r.Method != "POST" || r.URL.Path != "/v2/users/user/messages" || body["msg_type"] != float64(0) || body["msg_seq"] != float64(1) || body["msg_id"] != "message" || body["content"] != "pong" {
			t.Errorf("incorrect signed-callback HTTP request: %+v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"sent","timestamp":"2026-07-21T10:30:00+08:00"}`)
	}))
	defer server.Close()
	bm := core.NewBotManager()
	defer bm.StopAll()
	b, err := core.NewBot(config.BotConfig{ID: "test", AppID: "app", AppSecret: "secret", Mode: "webhook"})
	if err != nil {
		t.Fatal(err)
	}
	b.TokenSource = oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "fake", TokenType: "QQBot"})
	b.OpenAPI = botgo.NewOpenAPI(b.AppID, b.TokenSource)
	b.OpenAPI = b.OpenAPI.(interface{ WithBaseURL(string) openapi.OpenAPI }).WithBaseURL(server.URL)
	if err = bm.AddBot(b); err != nil {
		t.Fatal(err)
	}
	if err = b.Start(); err != nil {
		t.Fatal(err)
	}
	bm.Events().Subscribe(events.Filter{}, func(ctx context.Context, e events.Event) error {
		return e.Reply(ctx, events.Reply{Kind: "text", Text: "pong"})
	})
	routes := mux.NewRouter()
	routes.Handle("/webhook/{botID}", NewHandler(bm))
	raw := `{"op":0,"id":"envelope","t":"C2C_MESSAGE_CREATE","d":{"id":"message","content":"/ping","author":{"user_openid":"user"}}}`
	for range 2 {
		req := httptest.NewRequest("POST", "/webhook/test", strings.NewReader(raw))
		req.Header.Set(signature.HeaderTimestamp, "12345")
		sig, err := signature.Generate("secret", req.Header, []byte(raw))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set(signature.HeaderSig, sig)
		w := httptest.NewRecorder()
		routes.ServeHTTP(w, req)
		if w.Code != 200 || !strings.Contains(w.Body.String(), `"d":0`) {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if calls.Load() != 1 {
		t.Fatal("signed duplicate triggered sends", calls.Load())
	}
}
