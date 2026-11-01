package sdk_test

import (
	"context"
	"errors"
	"fmt"
	"shizukubot/sdk"
	"testing"
)

type host struct {
	commands  map[string]sdk.CommandHandler
	listeners map[sdk.EventType][]sdk.EventHandler
}

func newHost() *host {
	return &host{commands: map[string]sdk.CommandHandler{}, listeners: map[sdk.EventType][]sdk.EventHandler{}}
}
func (*host) Context() context.Context { return context.Background() }
func (*host) BotID() string            { return "unit" }
func (*host) Config() map[string]any   { return map[string]any{} }
func (*host) State() sdk.State         { return nil }
func (*host) Log(string)               {}
func (h *host) Command(name string, f sdk.CommandHandler) error {
	if _, ok := h.commands[name]; ok {
		return errors.New("duplicate command")
	}
	h.commands[name] = f
	return nil
}
func (h *host) On(typ sdk.EventType, f sdk.EventHandler) error {
	h.listeners[typ] = append(h.listeners[typ], f)
	return nil
}
func meta() sdk.Meta                       { return sdk.Meta{ID: "test", Name: "Test", Version: "1.0.0"} }
func noop(context.Context, sdk.Call) error { return nil }
func load(t *testing.T, d sdk.Definition) (sdk.Extension, *host) {
	t.Helper()
	if err := d.Validate(); err != nil {
		t.Fatal(err)
	}
	extension := d.New()
	h := newHost()
	if err := extension.Load(context.Background(), h); err != nil {
		t.Fatal(err)
	}
	return extension, h
}

func TestCommandPluginDefaultsAndLifecycle(t *testing.T) {
	d := sdk.CommandPlugin(meta(), "ping", func(ctx context.Context, c sdk.Call) error { return c.Reply(ctx, "pong") })
	if err := d.Validate(); err != nil {
		t.Fatal(err)
	}
	m := d.Manifest
	if m.APIVersion != sdk.APIVersion || m.Runtime != "go" || m.ID != "test" || m.ConfigSchema.Type != "object" {
		t.Fatal(m)
	}
	if len(m.Permissions) != 2 || !m.Allows("commands.register") || !m.Allows("message.send") {
		t.Fatal(m.Permissions)
	}
	if config, err := m.ConfigSchema.Validate(nil); err != nil || len(config.(map[string]any)) != 0 {
		t.Fatal(config, err)
	}
	extension, h := load(t, d)
	if extension == d.New() {
		t.Fatal("constructor shared an instance")
	}
	var reply string
	c := sdk.Call{Event: sdk.Event{Reply: func(_ context.Context, r sdk.Reply) error { reply = r.Text; return nil }}}
	if err := h.commands["ping"](context.Background(), c); err != nil || reply != "pong" {
		t.Fatal(reply, err)
	}
	for _, hook := range []func(context.Context) error{extension.Start, extension.Stop, extension.Unload} {
		if err := hook(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if err := extension.HandleEvent(context.Background(), c); err != nil {
		t.Fatal(err)
	}
}

func TestMetaValidationRequiresExplicitIdentity(t *testing.T) {
	for _, m := range []sdk.Meta{
		{}, {ID: "test", Name: "Test"}, {ID: "bad/id", Name: "Test", Version: "1.0.0"},
		{ID: "test", Version: "1.0.0"}, {ID: "test", Name: "  ", Version: "1.0.0"},
		{ID: "test", Name: "Test", Version: "latest"},
	} {
		if m.Validate() == nil {
			t.Fatal("invalid metadata accepted", m)
		}
		if sdk.CommandPlugin(m, "ping", noop).Validate() == nil {
			t.Fatal("definition hid metadata error", m)
		}
	}
	m := meta()
	m.Version = "1.2.3-beta.1"
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestEventPluginTypesAndConfiguration(t *testing.T) {
	types := []sdk.EventType{sdk.GroupMessage, sdk.GroupAtMessage, sdk.C2CMessage}
	calls := 0
	d := sdk.EventPlugin(meta(), types, func(context.Context, sdk.Call) error { calls++; return nil },
		sdk.WithConfig(sdk.Schema{Type: "object", Properties: map[string]sdk.Schema{"keyword": {Type: "string", Default: []byte(`"hello"`)}}}),
		sdk.WithPermissions("log.write", "config.read"),
	)
	types[0] = "changed"
	_, h := load(t, d)
	for _, typ := range []sdk.EventType{sdk.GroupMessage, sdk.GroupAtMessage, sdk.C2CMessage} {
		if len(h.listeners[typ]) != 1 {
			t.Fatal("missing canonical event", typ, h.listeners)
		}
		if err := h.listeners[typ][0](context.Background(), sdk.Call{}); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 3 || len(h.listeners) != 3 {
		t.Fatal(calls, h.listeners)
	}
	if len(d.Manifest.Permissions) != 4 || !d.Manifest.Allows("config.read") || d.Manifest.Allows("state.write") {
		t.Fatal(d.Manifest.Permissions)
	}
	value, err := d.Manifest.ConfigSchema.Validate(nil)
	if err != nil || value.(map[string]any)["keyword"] != "hello" {
		t.Fatal(value, err)
	}
	invalid := sdk.CommandPlugin(meta(), "ping", noop, sdk.WithPermissions("filesystem.all"))
	if invalid.Validate() == nil {
		t.Fatal("unknown extra capability accepted")
	}
}

func TestDefineIndependentClosuresAndSealedRegistrar(t *testing.T) {
	var retained sdk.Registrar
	d := sdk.Define(meta(), func(r sdk.Registrar) error {
		retained = r
		count := 0
		if err := r.Command("count", func(ctx context.Context, c sdk.Call) error { count++; return c.Reply(ctx, fmt.Sprint(count)) }); err != nil {
			return err
		}
		return r.On(sdk.C2CMessage, noop)
	})
	_, a := load(t, d)
	ra := retained
	_, b := load(t, d)
	if err := ra.Command("late", noop); err == nil {
		t.Fatal("registration after Load accepted")
	}
	if len(a.commands) != 1 || len(b.commands) != 1 {
		t.Fatal(a.commands, b.commands)
	}
	for _, tc := range []struct {
		h    *host
		want string
	}{{a, "1"}, {a, "2"}, {b, "1"}} {
		got := ""
		err := tc.h.commands["count"](context.Background(), sdk.Call{Event: sdk.Event{Reply: func(_ context.Context, r sdk.Reply) error { got = r.Text; return nil }}})
		if err != nil || got != tc.want {
			t.Fatal(got, tc.want, err)
		}
	}
}

func TestRegistrationErrorsAreReturnedWithoutPanicking(t *testing.T) {
	definitions := []sdk.Definition{
		sdk.CommandPlugin(meta(), "invalid command", noop), sdk.CommandPlugin(meta(), "ping", nil),
		sdk.EventPlugin(meta(), nil, noop), sdk.EventPlugin(meta(), []sdk.EventType{""}, noop),
		sdk.EventPlugin(meta(), []sdk.EventType{sdk.C2CMessage}, nil), sdk.Define(meta(), nil),
		sdk.Define(meta(), func(r sdk.Registrar) error {
			if err := r.Command("same", noop); err != nil {
				return err
			}
			_ = r.Command("same", noop) // Even an accidentally ignored failure must fail Load.
			return nil
		}),
	}
	for i, d := range definitions {
		if err := d.New().Load(context.Background(), newHost()); err == nil {
			t.Fatal("registration error lost", i)
		}
	}
	expected := errors.New("setup failed")
	d := sdk.Define(meta(), func(sdk.Registrar) error { return expected })
	if err := d.New().Load(context.Background(), newHost()); !errors.Is(err, expected) {
		t.Fatal(err)
	}
}

func TestReplyHelpersUseOneCapabilityAndPropagateErrors(t *testing.T) {
	captured := []sdk.Reply{}
	expected := errors.New("source does not support media")
	c := sdk.Call{Event: sdk.Event{BotID: "bot", UserID: "user", GroupID: "group", Message: "hello", Reply: func(_ context.Context, r sdk.Reply) error { captured = append(captured, r); return expected }}}
	for _, call := range []func() error{
		func() error { return c.Reply(context.Background(), "text") },
		func() error {
			return c.Send(context.Background(), sdk.Reply{Kind: "video", URL: "https://example.com/video"})
		},
		func() error { return c.ReplyMarkdown(context.Background(), "**hello**") },
		func() error { return c.ReplyImage(context.Background(), "https://example.com/image") },
	} {
		if err := call(); !errors.Is(err, expected) {
			t.Fatal("capability error hidden", err)
		}
	}
	if len(captured) != 4 || captured[0].Kind != "text" || captured[1].Kind != "video" || captured[2].Kind != "markdown" || captured[2].Text != "**hello**" || captured[3].Kind != "image" {
		t.Fatal(captured)
	}
	if c.Event.BotID != "bot" || c.Event.UserID != "user" || c.Event.GroupID != "group" || c.Event.Message != "hello" {
		t.Fatal("event identity changed")
	}
	if err := (sdk.Call{}).ReplyImage(context.Background(), "image"); err == nil {
		t.Fatal("missing sender was ignored")
	}
}
