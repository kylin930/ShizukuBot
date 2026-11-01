package webhook

import (
	"context"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/tencent-connect/botgo/interaction/signature"
	"golang.org/x/oauth2"
	"net/http"
	"net/http/httptest"
	"shizukubot/config"
	"shizukubot/core"
	"shizukubot/events"
	"strings"
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
