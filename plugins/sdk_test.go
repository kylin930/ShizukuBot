package plugins_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"shizukubot/events"
	"shizukubot/plugins"
	"shizukubot/sdk"
	"strings"
	"testing"
)

func manager(t *testing.T, definitions ...sdk.Definition) *plugins.Manager {
	t.Helper()
	registry := plugins.NewRegistry()
	for _, d := range definitions {
		if err := registry.Register(d); err != nil {
			t.Fatal(err)
		}
	}
	m, err := plugins.NewManager(registry, events.NewBus(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := m.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})
	return m
}
func metadata(id string) sdk.Meta { return sdk.Meta{ID: id, Name: id, Version: "1.0.0"} }
func publish(m *plugins.Manager, bot, message string) ([]string, error) {
	replies := []string{}
	err := m.Bus.Publish(context.Background(), sdk.Event{BotID: bot, Type: sdk.C2CMessage, Source: events.C2C, UserID: "u", Message: message, Reply: func(_ context.Context, r sdk.Reply) error { replies = append(replies, r.Text); return nil }})
	return replies, err
}
func enable(t *testing.T, m *plugins.Manager, id, bot string) {
	t.Helper()
	if err := m.Enable(context.Background(), id, bot); err != nil {
		t.Fatal(err)
	}
}

func TestSimpleManagerBotIsolationAndToggle(t *testing.T) {
	d := sdk.Define(metadata("simple"), func(r sdk.Registrar) error {
		n := 0
		return r.Command("count", func(ctx context.Context, c sdk.Call) error {
			n++
			return c.Reply(ctx, fmt.Sprintf("%s:%d", c.Event.BotID, n))
		})
	})
	m := manager(t, d)
	enable(t, m, "simple", "a")
	enable(t, m, "simple", "b")
	enable(t, m, "simple", "a")
	for _, tc := range []struct{ bot, want string }{{"a", "a:1"}, {"a", "a:2"}, {"b", "b:1"}} {
		replies, err := publish(m, tc.bot, "/count")
		if err != nil || len(replies) != 1 || replies[0] != tc.want {
			t.Fatal(replies, err)
		}
	}
	if err := m.Disable(context.Background(), "simple", "a"); err != nil {
		t.Fatal(err)
	}
	if replies, err := publish(m, "a", "/count"); err != nil || len(replies) != 0 {
		t.Fatal(replies, err)
	}
	if err := m.Restart(context.Background(), "simple", "b"); err != nil {
		t.Fatal(err)
	}
	enable(t, m, "simple", "a")
	for _, bot := range []string{"a", "b"} {
		replies, err := publish(m, bot, "/count")
		if err != nil || len(replies) != 1 || replies[0] != bot+":1" {
			t.Fatal(replies, err)
		}
	}
}

func TestSimpleEventsAndBotConfigurationUseNormalBus(t *testing.T) {
	d := sdk.EventPlugin(metadata("configured"), []sdk.EventType{sdk.GroupMessage, sdk.GroupAtMessage, sdk.C2CMessage}, func(ctx context.Context, c sdk.Call) error {
		return c.Reply(ctx, c.Config["reply"].(string))
	}, sdk.WithConfig(sdk.Schema{Type: "object", Properties: map[string]sdk.Schema{"reply": {Type: "string", Default: []byte(`"default"`)}}}))
	m := manager(t, d)
	for _, bot := range []string{"a", "b"} {
		if err := m.Configure("configured", bot, map[string]any{"reply": bot}); err != nil {
			t.Fatal(err)
		}
		enable(t, m, "configured", bot)
	}
	for _, bot := range []string{"a", "b", "unsubscribed"} {
		for _, typ := range []sdk.EventType{sdk.GroupMessage, sdk.GroupAtMessage, sdk.C2CMessage, sdk.GuildMessage} {
			replies := []string{}
			err := m.Bus.Publish(context.Background(), sdk.Event{BotID: bot, Type: typ, Reply: func(_ context.Context, r sdk.Reply) error { replies = append(replies, r.Text); return nil }})
			want := 1
			if bot == "unsubscribed" || typ == sdk.GuildMessage {
				want = 0
			}
			if err != nil || len(replies) != want || want == 1 && replies[0] != bot {
				t.Fatal(bot, typ, replies, err)
			}
		}
	}
}

func TestSimpleCapabilitiesCannotBypassHostChecks(t *testing.T) {
	denial := errors.New("message.send denied")
	d := sdk.CommandPlugin(metadata("guard"), "guard", func(ctx context.Context, c sdk.Call) error {
		if len(c.Config) != 0 {
			t.Fatal("configuration granted by default")
		}
		if _, err := c.State.Get(ctx, "secret"); err == nil {
			t.Fatal("state granted by default")
		}
		for _, call := range []func() error{
			func() error { return c.Reply(ctx, "text") }, func() error { return c.ReplyMarkdown(ctx, "md") },
			func() error { return c.ReplyImage(ctx, "https://example.com/image") },
			func() error { return c.Send(ctx, sdk.Reply{Kind: "video"}) },
			func() error { return c.Event.Reply(ctx, sdk.Reply{Kind: "text"}) },
		} {
			if err := call(); err == nil || err.Error() != denial.Error() {
				t.Fatal("send bypass", err)
			}
		}
		c.Log("denied log")
		return nil
	})
	d.Manifest.Permissions = []string{"commands.register"}
	m := manager(t, d)
	m.Log = func(string, string, string) { t.Fatal("log capability granted by default") }
	enable(t, m, "guard", "a")
	if replies, err := publish(m, "a", "/guard"); err != nil || len(replies) != 0 {
		t.Fatal(replies, err)
	}
}

type advanced struct {
	sdk.Base
	starts, stops *int
}

func (a *advanced) Load(_ context.Context, h sdk.Host) error {
	return h.Command("run", func(ctx context.Context, c sdk.Call) error { return c.Reply(ctx, "advanced") })
}
func (a *advanced) Start(context.Context) error { *a.starts++; return nil }
func (a *advanced) Stop(context.Context) error  { *a.stops++; return nil }
func TestSimplePanicIsIsolatedFromAdvancedPlugin(t *testing.T) {
	starts, stops := 0, 0
	advancedDefinition := sdk.Definition{Manifest: sdk.Manifest{APIVersion: sdk.APIVersion, Runtime: "go", ID: "advanced", Name: "Advanced", Version: "1.0.0", Permissions: []string{"commands.register", "message.send"}, ConfigSchema: sdk.Schema{Type: "object"}}, New: func() sdk.Extension { return &advanced{starts: &starts, stops: &stops} }}
	fault := sdk.CommandPlugin(metadata("fault"), "run", func(context.Context, sdk.Call) error { panic("simple failure") })
	m := manager(t, fault, advancedDefinition)
	enable(t, m, "fault", "a")
	enable(t, m, "advanced", "a")
	replies, err := publish(m, "a", "/run")
	if err == nil || !strings.Contains(err.Error(), "plugin panic") || len(replies) != 1 || replies[0] != "advanced" {
		t.Fatal(replies, err)
	}
	if err = m.Disable(context.Background(), "advanced", "a"); err != nil {
		t.Fatal(err)
	}
	if starts != 1 || stops != 1 {
		t.Fatal(starts, stops)
	}
}

func TestSimpleRegistrationFailureIsReportedByManager(t *testing.T) {
	d := sdk.Define(metadata("invalid"), func(r sdk.Registrar) error {
		if err := r.Command("duplicate", func(context.Context, sdk.Call) error { return nil }); err != nil {
			return err
		}
		_ = r.Command("duplicate", func(context.Context, sdk.Call) error { return nil })
		return nil
	})
	m := manager(t, d)
	if err := m.Enable(context.Background(), "invalid", "a"); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatal(err)
	}
	statuses := m.Statuses()
	if len(statuses) != 1 || statuses[0].Enabled || statuses[0].State != "error" || statuses[0].LastError == "" {
		t.Fatal(statuses)
	}
	if commands := m.Commands(nil, "a"); len(commands) != 0 {
		t.Fatal(commands)
	}
}

func TestSimpleDebugUsesTemporaryStateAndCurrentBot(t *testing.T) {
	d := sdk.CommandPlugin(metadata("stateful"), "count", func(ctx context.Context, c sdk.Call) error {
		raw, err := c.State.Get(ctx, "count")
		if err != nil {
			return err
		}
		var n int
		if len(raw) > 0 {
			if err = json.Unmarshal(raw, &n); err != nil {
				return err
			}
		}
		n++
		raw, err = json.Marshal(n)
		if err != nil {
			return err
		}
		if err = c.State.Set(ctx, "count", raw); err != nil {
			return err
		}
		return c.Reply(ctx, fmt.Sprintf("%s:%d", c.Event.BotID, n))
	}, sdk.WithPermissions("state.read", "state.write"))
	m := manager(t, d)
	enable(t, m, "stateful", "a")
	if replies, err := publish(m, "a", "/count"); err != nil || len(replies) != 1 || replies[0] != "a:1" {
		t.Fatal(replies, err)
	}
	for range 2 {
		replies := []string{}
		err := m.Debug(context.Background(), "stateful", sdk.Event{BotID: "a", Type: sdk.C2CMessage, Message: "/count", Reply: func(_ context.Context, r sdk.Reply) error { replies = append(replies, r.Text); return nil }}, nil)
		if err != nil || len(replies) != 1 || replies[0] != "a:1" {
			t.Fatal("debug state leaked", replies, err)
		}
	}
	if replies, err := publish(m, "a", "/count"); err != nil || len(replies) != 1 || replies[0] != "a:2" {
		t.Fatal("production state changed by debug", replies, err)
	}
}
