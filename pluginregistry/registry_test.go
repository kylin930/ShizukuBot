package pluginregistry

import (
	"context"
	"shizukubot/events"
	"shizukubot/plugins"
	"strings"
	"testing"
)

func TestCompiledExamplesAcrossBotsAndDebug(t *testing.T) {
	registry, err := New()
	if err != nil {
		t.Fatal(err)
	}
	bus := events.NewBus()
	manager, err := plugins.NewManager(registry, bus, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := manager.Close(context.Background()); err != nil {
			t.Error(err)
		}
	}()
	list, err := manager.Packages()
	if err != nil || len(list) != 4 {
		t.Fatal(list, err)
	}
	for _, bot := range []string{"a", "b"} {
		for _, p := range list {
			if err = manager.Enable(context.Background(), p.Manifest.ID, bot); err != nil {
				t.Fatal(err)
			}
		}
	}
	publish := func(bot, message string) []string {
		t.Helper()
		out := []string{}
		if err := bus.Publish(context.Background(), events.Event{BotID: bot, Type: "c2c_message", Source: events.C2C, UserID: "u", Message: message, Reply: func(_ context.Context, r events.Reply) error { out = append(out, r.Text); return nil }}); err != nil {
			t.Fatal(err)
		}
		return out
	}
	for _, tc := range []struct{ bot, message, want string }{{"a", "/ping", "pong"}, {"a", "你好", "你好！"}, {"a", "/count", "计数: 1"}, {"a", "/count", "计数: 2"}, {"b", "/count", "计数: 1"}} {
		got := publish(tc.bot, tc.message)
		if len(got) != 1 || got[0] != tc.want {
			t.Fatal(tc, got)
		}
	}
	if err = manager.Configure("autoreply", "b", map[string]any{"keyword": "hello", "reply": "world"}); err != nil {
		t.Fatal(err)
	}
	if err = manager.Restart(context.Background(), "autoreply", "b"); err != nil {
		t.Fatal(err)
	}
	if got := publish("b", "hello"); len(got) != 1 || got[0] != "world" {
		t.Fatal(got)
	}
	if got := publish("a", "hello"); len(got) != 0 {
		t.Fatal("config crossed Bot scope", got)
	}
	got := publish("a", "/framework")
	if len(got) != 1 || !strings.Contains(got[0], "ShizukuBot") {
		t.Fatal(got)
	}
}
