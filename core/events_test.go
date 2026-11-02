package core

import (
	"context"
	"errors"
	"github.com/tencent-connect/botgo/dto"
	"github.com/tencent-connect/botgo/event"
	"github.com/tencent-connect/botgo/openapi"
	"github.com/tencent-connect/botgo/openapi/options"
	"golang.org/x/oauth2"
	"shizukubot/config"
	"shizukubot/events"
	"shizukubot/sdk"
	"strings"
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

type canceledAPI struct {
	openapi.OpenAPI
	entered chan struct{}
}

func (a *canceledAPI) PostGroupMessage(ctx context.Context, _ string, _ dto.APIMessage, _ ...options.Option) (*dto.Message, error) {
	close(a.entered)
	<-ctx.Done()
	return nil, ctx.Err()
}
func TestReplyPropagatesCancellation(t *testing.T) {
	b := testBot(t, "cancel")
	api := &canceledAPI{entered: make(chan struct{})}
	b.OpenAPI = api
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- b.reply(events.Event{Type: "group_at_message", Source: events.Group, GroupID: "g", MessageID: "message"})(ctx, events.Reply{Kind: "text", Text: "hello"})
	}()
	<-api.entered
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("reply ignored cancellation")
	}
}

type blockingToken struct{ entered, release chan struct{} }

func (b blockingToken) Token() (*oauth2.Token, error) {
	close(b.entered)
	<-b.release
	return &oauth2.Token{AccessToken: "mock", TokenType: "QQBot"}, nil
}
func TestAuthenticationDoesNotHoldBotStateLock(t *testing.T) {
	b := testBot(t, "auth-lock")
	entered, release := make(chan struct{}), make(chan struct{})
	b.TokenSource = blockingToken{entered, release}
	done := make(chan error, 1)
	go func() { done <- b.Start() }()
	<-entered
	queried := make(chan struct{})
	go func() { b.ConfigSnapshot(); close(queried) }()
	select {
	case <-queried:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("authentication blocked state query")
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	b.Stop()
}

// SDK conveniences delegate unsupported-source errors to the real core adapter.
// No QQ client is initialized or called for these unsupported media requests.
func TestSDKMediaHelpersRejectUnsupportedSources(t *testing.T) {
	bot := testBot(t, "media")
	for _, source := range []events.Source{events.Guild, events.Direct} {
		e := sdk.Event{BotID: bot.ID, Source: source, ChannelID: "channel", GuildID: "guild"}
		e.Reply = bot.reply(e)
		call := sdk.Call{Event: e}
		for _, send := range []func() error{
			func() error { return call.ReplyImage(context.Background(), "https://example.com/image") },
			func() error { return call.ReplyMarkdown(context.Background(), "**markdown**") },
		} {
			if err := send(); err == nil || !strings.Contains(err.Error(), "unsupported") {
				t.Fatal(source, err)
			}
		}
	}
}
