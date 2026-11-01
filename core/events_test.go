package core

import (
	"context"
	"github.com/tencent-connect/botgo/dto"
	"github.com/tencent-connect/botgo/event"
	"golang.org/x/oauth2"
	"shizukubot/config"
	"shizukubot/events"
	"sync"
	"testing"
	"time"
)

func testBot(t *testing.T, id string) *Bot {
	t.Helper()
	b, err := NewBot(config.BotConfig{ID: id, AppID: "test-app", AppSecret: "test-secret", Mode: "webhook", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	b.TokenSource = oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "mock", TokenType: "QQBot"})
	return b
}
func TestBotIsolationRestartAndLifecycle(t *testing.T) {
	bm := NewBotManager()
	a, b := testBot(t, "a"), testBot(t, "b")
	bm.AddBot(a)
	bm.AddBot(b)
	// Installing a hostile global SDK callback must have no effect on application dispatch.
	old := event.DefaultHandlers.C2CMessage
	defer func() { event.DefaultHandlers.C2CMessage = old }()
	event.RegisterHandlers(event.C2CMessageEventHandler(func(*dto.WSPayload, *dto.WSC2CMessageData) error { t.Error("used global callback"); return nil }))
	counts := map[string]int{}
	var mu sync.Mutex
	off := bm.Events().Subscribe(events.Filter{}, func(_ context.Context, e events.Event) error {
		mu.Lock()
		defer mu.Unlock()
		counts[e.BotID]++
		return nil
	})
	defer off()
	if err := bm.StartAll(); err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{"op":0,"t":"C2C_MESSAGE_CREATE","d":{"id":"m","content":"same","author":{"user_openid":"u"}}}`)
	for _, bot := range []*Bot{a, b} {
		if err := bot.HandlePayload(context.Background(), raw); err != nil {
			t.Fatal(err)
		}
	}
	if counts["a"] != 1 || counts["b"] != 1 {
		t.Fatal(counts)
	}
	a.Stop()
	if a.IsRunning() {
		t.Fatal("still running")
	}
	if a.HandlePayload(context.Background(), raw) == nil {
		t.Fatal("stopped bot accepted event")
	}
	if err := a.Start(); err != nil {
		t.Fatal(err)
	}
	next := []byte(`{"op":0,"t":"C2C_MESSAGE_CREATE","d":{"id":"m2","content":"same","author":{"user_openid":"u"}}}`)
	if err := a.HandlePayload(context.Background(), next); err != nil {
		t.Fatal(err)
	}
	if counts["a"] != 2 {
		t.Fatal("restart duplicated subscription", counts)
	}
	bm.StopAll()
	bm.StopAll()
}
func TestBotStartFailureCancels(t *testing.T) {
	b := testBot(t, "a")
	b.TokenSource = failingToken{}
	if b.Start() == nil || b.ctx.Err() == nil || b.IsRunning() {
		t.Fatal("failed lifecycle was retained")
	}
	b.Stop()
}

type failingToken struct{}

func (failingToken) Token() (*oauth2.Token, error) { return nil, context.DeadlineExceeded }

func TestRemoveBotCancelsInflightHandlerWithoutManagerDeadlock(t *testing.T) {
	bm := NewBotManager()
	b := testBot(t, "remove")
	if err := bm.AddBot(b); err != nil {
		t.Fatal(err)
	}
	if err := b.Start(); err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	bm.Events().Subscribe(events.Filter{}, func(ctx context.Context, e events.Event) error {
		close(entered)
		<-ctx.Done()
		bm.GetBot(e.BotID)
		return ctx.Err()
	})
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		b.HandlePayload(context.Background(), []byte(`{"op":0,"t":"C2C_MESSAGE_CREATE","d":{"id":"m"}}`))
	}()
	<-entered
	removed := make(chan error, 1)
	go func() { removed <- bm.RemoveBot(b.ID) }()
	select {
	case err := <-removed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("remove deadlocked with event handler")
	}
	<-finished
}

func TestBuiltinReplyDoesNotHoldBotLock(t *testing.T) {
	b := testBot(t, "lock")
	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		b.HandleBuiltinCommand(context.Background(), events.Event{Message: "#shizuku", Reply: func(context.Context, events.Reply) error { close(entered); <-release; return nil }})
	}()
	<-entered
	snapshot := make(chan struct{})
	go func() { b.ConfigSnapshot(); close(snapshot) }()
	select {
	case <-snapshot:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("network reply held bot mutex")
	}
	close(release)
	<-done
}
