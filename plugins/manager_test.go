package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"shizukubot/events"
	"strings"
	"sync"
	"testing"
	"time"
)

type callbacks struct {
	Base
	load   func(context.Context, Host) error
	stop   func(context.Context) error
	unload func(context.Context) error
}

func (e *callbacks) Load(ctx context.Context, h Host) error {
	if e.load != nil {
		return e.load(ctx, h)
	}
	return nil
}
func (e *callbacks) Stop(ctx context.Context) error {
	if e.stop != nil {
		return e.stop(ctx)
	}
	return nil
}
func (e *callbacks) Unload(ctx context.Context) error {
	if e.unload != nil {
		return e.unload(ctx)
	}
	return nil
}
func definition(id string, newPlugin func() Extension) Definition {
	return Definition{Manifest: Manifest{APIVersion: 1, ID: id, Name: id, Version: "1.0.0", Runtime: "go", Permissions: []string{"commands.register", "events.subscribe", "message.send", "config.read", "state.read", "state.write", "log.write"}, ConfigSchema: Schema{Type: "object"}}, New: newPlugin}
}
func testManager(t *testing.T, defs ...Definition) *Manager {
	t.Helper()
	r := NewRegistry()
	for _, d := range defs {
		if err := r.Register(d); err != nil {
			t.Fatal(err)
		}
	}
	m, err := NewManager(r, events.NewBus(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := m.Close(ctx); err != nil {
			t.Error(err)
		}
	})
	return m
}
func emit(m *Manager, bot, id, message string) ([]string, error) {
	out := []string{}
	err := m.Bus.Publish(context.Background(), events.Event{BotID: bot, Type: "group_message", Source: events.Group, MessageID: id, GroupID: "g", UserID: "u", Message: message, Reply: func(_ context.Context, r events.Reply) error { out = append(out, r.Text); return nil }})
	return out, err
}
func TestRegistryValidationAndSnapshot(t *testing.T) {
	d := definition("native", func() Extension { return &Base{} })
	r := NewRegistry()
	if err := r.Register(d); err != nil {
		t.Fatal(err)
	}
	if r.Register(d) == nil {
		t.Fatal("duplicate accepted")
	}
	d.Manifest.ID = "script"
	d.Manifest.Runtime = "javascript"
	if r.Register(d) == nil {
		t.Fatal("script runtime accepted")
	}
	d = definition("invalid", nil)
	if r.Register(d) == nil {
		t.Fatal("nil factory accepted")
	}
	invalid := definition("invalid", func() Extension { return &Base{} })
	invalid.Manifest.Permissions = []string{"unknown.permission"}
	if r.Register(invalid) == nil {
		t.Fatal("invalid permission accepted")
	}
	invalid = definition("invalid", func() Extension { return &Base{} })
	invalid.Manifest.ConfigSchema = Schema{Type: "array"}
	if r.Register(invalid) == nil {
		t.Fatal("non-object schema accepted")
	}
	invalid = definition("invalid", func() Extension { return &Base{} })
	invalid.Manifest.ConfigSchema = Schema{Type: "object", Enum: []any{func() {}}}
	if r.Register(invalid) == nil {
		t.Fatal("non-JSON manifest accepted")
	}
	m, err := NewManager(r, events.NewBus(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close(context.Background())
	if err = r.Register(definition("late", func() Extension { return &Base{} })); err != nil {
		t.Fatal(err)
	}
	if m.Enable(context.Background(), "late", "a") == nil {
		t.Fatal("registry changed after manager creation")
	}
}
func TestGoInstancesIsolationAndLifecycle(t *testing.T) {
	var all []*extension
	m := testManager(t, definition("native", func() Extension { e := &extension{}; all = append(all, e); return e }))
	for _, bot := range []string{"a", "b"} {
		if err := m.Enable(context.Background(), "native", bot); err != nil {
			t.Fatal(err)
		}
	}
	if err := m.Enable(context.Background(), "native", "a"); err != nil {
		t.Fatal(err)
	}
	emit(m, "a", "one", "/ping")
	emit(m, "b", "one", "/ping")
	emit(m, "c", "one", "/ping")
	if len(all) != 2 || all[0].calls != 1 || all[1].calls != 1 {
		t.Fatal(all)
	}
	if err := m.Restart(context.Background(), "native", "a"); err != nil {
		t.Fatal(err)
	}
	if all[0].stop != 1 || all[0].unload != 1 || len(all) != 3 {
		t.Fatal(all)
	}
	emit(m, "a", "two", "/ping")
	if all[2].calls != 1 {
		t.Fatal("duplicate subscription")
	}
	if err := m.Disable(context.Background(), "native", "a"); err != nil {
		t.Fatal(err)
	}
	emit(m, "a", "three", "/ping")
	if all[2].calls != 1 || all[2].stop != 1 {
		t.Fatal("disabled plugin executed")
	}
}
func TestErrorsPanicsAndMultipleHandlers(t *testing.T) {
	observed := 0
	d := definition("fault", func() Extension {
		return &callbacks{load: func(_ context.Context, h Host) error {
			for _, handler := range []func(context.Context, Call) error{func(context.Context, Call) error { return errors.New("failure") }, func(context.Context, Call) error { panic("fixture") }, func(context.Context, Call) error { observed++; return nil }} {
				if err := h.On("*", handler); err != nil {
					return err
				}
			}
			return nil
		}}
	})
	m := testManager(t, d, definition("healthy", func() Extension {
		return &callbacks{load: func(_ context.Context, h Host) error {
			return h.On("*", func(context.Context, Call) error { observed++; return nil })
		}}
	}))
	for _, id := range []string{"fault", "healthy"} {
		if err := m.Enable(context.Background(), id, "a"); err != nil {
			t.Fatal(err)
		}
	}
	for range 2 {
		if _, err := emit(m, "a", "", "hello"); err == nil || !strings.Contains(err.Error(), "plugin panic") {
			t.Fatal(err)
		}
	}
	if observed != 4 {
		t.Fatal("error interrupted other handlers", observed)
	}
	if m.Statuses()[0].State != "error" {
		t.Fatal(m.Statuses())
	}
}
func TestPermissionsAndConfigValidation(t *testing.T) {
	denied := 0
	d := definition("guard", func() Extension {
		return &callbacks{load: func(ctx context.Context, h Host) error {
			if h.BotID() != "a" || len(h.Config()) != 0 {
				t.Fatal("host capability leak")
			}
			if h.State().Set(ctx, "key", json.RawMessage(`1`)) == nil {
				t.Fatal("host state permission ignored")
			}
			return h.Command("guard", func(ctx context.Context, c Call) error {
				if c.Reply(ctx, "forbidden") == nil {
					t.Fatal("send permission ignored")
				}
				if _, err := c.State.Get(ctx, "key"); err == nil {
					t.Fatal("read permission ignored")
				}
				denied++
				return nil
			})
		}}
	})
	d.Manifest.Permissions = []string{"commands.register"}
	m := testManager(t, d)
	if err := m.Enable(context.Background(), "guard", "a"); err != nil {
		t.Fatal(err)
	}
	out, err := emit(m, "a", "", "/guard")
	if err != nil || len(out) != 0 || denied != 1 {
		t.Fatal(out, err, denied)
	}
	if m.Configure("guard", "a", map[string]any{"unknown": true}) == nil {
		t.Fatal("unknown config accepted")
	}
	if m.Configure("guard", "a", map[string]any{"bad": func() {}}) == nil {
		t.Fatal("non-JSON accepted")
	}
	if m.Configure("guard", "", nil) == nil {
		t.Fatal("empty scope accepted")
	}
}
func TestStateConfigBackupRestoreAndSandboxIsolation(t *testing.T) {
	d := definition("stateful", func() Extension {
		return &callbacks{load: func(_ context.Context, h Host) error {
			return h.Command("count", func(ctx context.Context, c Call) error {
				raw, err := c.State.Get(ctx, "n")
				if err != nil {
					return err
				}
				n := 0
				if len(raw) > 0 {
					if err = json.Unmarshal(raw, &n); err != nil {
						return err
					}
				}
				n++
				raw, _ = json.Marshal(n)
				if err = c.State.Set(ctx, "n", raw); err != nil {
					return err
				}
				return c.Reply(ctx, c.Config["prefix"].(string)+string(raw))
			})
		}}
	})
	d.Manifest.ConfigSchema = Schema{Type: "object", Properties: map[string]Schema{"prefix": {Type: "string", Default: []byte(`"n="`)}}}
	m := testManager(t, d)
	for _, bot := range []string{"a", "b"} {
		if err := m.Configure("stateful", bot, map[string]any{"prefix": bot}); err != nil {
			t.Fatal(err)
		}
		if err := m.Enable(context.Background(), "stateful", bot); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct{ bot, want string }{{"a", "a1"}, {"a", "a2"}, {"b", "b1"}} {
		out, err := emit(m, tc.bot, "", "/count")
		if err != nil || len(out) != 1 || out[0] != tc.want {
			t.Fatal(out, err)
		}
	}
	debug := []string{}
	e := events.Event{BotID: "a", Message: "/count", Reply: func(_ context.Context, r events.Reply) error { debug = append(debug, r.Text); return nil }}
	for range 2 {
		if err := m.Debug(context.Background(), "stateful", e, nil); err != nil {
			t.Fatal(err)
		}
	}
	if strings.Join(debug, ",") != "a1,a1" {
		t.Fatal(debug)
	}
	if out, err := emit(m, "a", "", "/count"); err != nil || out[0] != "a3" {
		t.Fatal(out, err)
	}
	raw, err := m.ExportBackup("stateful")
	if err != nil {
		t.Fatal(err)
	}
	next := testManager(t, d)
	if err = next.RestoreBackup(raw); err != nil {
		t.Fatal(err)
	}
	if next.Config("stateful", "a").Enabled {
		t.Fatal("restore activated code")
	}
	if next.RestoreBackup(raw) == nil {
		t.Fatal("overwrote data")
	}
	if err = next.Enable(context.Background(), "stateful", "a"); err != nil {
		t.Fatal(err)
	}
	if out, err := emit(next, "a", "", "/count"); err != nil || out[0] != "a4" {
		t.Fatal(out, err)
	}
	bad := []byte(`{"version":1,"plugin_id":"unknown","bots":{}}`)
	if next.RestoreBackup(bad) == nil {
		t.Fatal("unknown plugin restore accepted")
	}
	if err = m.Configure("stateful", "a", map[string]any{"prefix": "new"}); err != nil {
		t.Fatal(err)
	}
	if err = m.Restart(context.Background(), "stateful", "a"); err != nil {
		t.Fatal(err)
	}
	if out, err := emit(m, "a", "", "/count"); err != nil || out[0] != "new4" {
		t.Fatal(out, err)
	}
}
func TestDisableAndCloseCancelInflight(t *testing.T) {
	for _, debug := range []bool{false, true} {
		t.Run(map[bool]string{false: "live", true: "debug"}[debug], func(t *testing.T) {
			entered := make(chan struct{})
			var once sync.Once
			cleaned := 0
			d := definition("cancel", func() Extension {
				return &callbacks{load: func(_ context.Context, h Host) error {
					return h.Command("wait", func(ctx context.Context, _ Call) error {
						once.Do(func() { close(entered) })
						<-ctx.Done()
						return ctx.Err()
					})
				}, stop: func(context.Context) error { cleaned++; return nil }}
			})
			m := testManager(t, d)
			m.Timeout = time.Second * 5
			done := make(chan error, 1)
			if debug {
				go func() {
					done <- m.Debug(context.Background(), "cancel", events.Event{BotID: "a", Message: "/wait"}, nil)
				}()
			} else {
				if err := m.Enable(context.Background(), "cancel", "a"); err != nil {
					t.Fatal(err)
				}
				go func() { _, err := emit(m, "a", "", "/wait"); done <- err }()
			}
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("not entered")
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			var err error
			if debug {
				err = m.Close(ctx)
			} else {
				err = m.Disable(ctx, "cancel", "a")
			}
			if err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-done:
				if !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
			case <-ctx.Done():
				t.Fatal("cancellation leaked execution")
			}
			if cleaned != 1 {
				t.Fatal(cleaned)
			}
			if debug && m.Debug(context.Background(), "cancel", events.Event{}, nil) == nil {
				t.Fatal("closed manager accepted debug")
			}
		})
	}
}
func TestStartFailureCleansResourcesAndCancelsLife(t *testing.T) {
	var life context.Context
	stopped, unloaded := 0, 0
	m := testManager(t, definition("failed", func() Extension {
		return &callbacks{load: func(_ context.Context, h Host) error { life = h.Context(); return errors.New("initialization failed") }, stop: func(context.Context) error {
			if life.Err() == nil {
				t.Fatal("life not canceled before stop")
			}
			stopped++
			return nil
		}, unload: func(context.Context) error { unloaded++; return nil }}
	}))
	if m.Enable(context.Background(), "failed", "a") == nil {
		t.Fatal("init error hidden")
	}
	if stopped != 1 || unloaded != 1 {
		t.Fatal(stopped, unloaded)
	}
}
func TestGoTimeoutIsCooperativeAndReported(t *testing.T) {
	m := testManager(t, definition("timeout", func() Extension {
		return &callbacks{load: func(_ context.Context, h Host) error {
			return h.Command("wait", func(ctx context.Context, _ Call) error { <-ctx.Done(); return nil })
		}}
	}))
	m.Timeout = 10 * time.Millisecond
	if err := m.Enable(context.Background(), "timeout", "a"); err != nil {
		t.Fatal(err)
	}
	if _, err := emit(m, "a", "", "/wait"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
}
func TestRestoreEnabledSettingsWithoutSharingInstances(t *testing.T) {
	all := []*extension{}
	d := definition("native", func() Extension { e := &extension{}; all = append(all, e); return e })
	m := testManager(t, d)
	if err := m.Enable(context.Background(), "native", "a"); err != nil {
		t.Fatal(err)
	}
	if err := m.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	registry := NewRegistry()
	if err := registry.Register(d); err != nil {
		t.Fatal(err)
	}
	next, err := NewManager(registry, events.NewBus(), m.root)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close(context.Background())
	if err = next.Restore(context.Background(), []string{"a", "b"}); err != nil {
		t.Fatal(err)
	}
	emit(next, "a", "", "/ping")
	emit(next, "b", "", "/ping")
	if len(all) != 2 || all[0].stop != 1 || all[1].calls != 1 {
		t.Fatal(all)
	}
	if _, err = next.ExportBackup("../escape"); err == nil {
		t.Fatal("invalid id exported")
	}
}

func TestBackgroundResourcesStopForLiveAndDebug(t *testing.T) {
	for _, debug := range []bool{false, true} {
		t.Run(map[bool]string{false: "live", true: "debug"}[debug], func(t *testing.T) {
			stopped := 0
			d := definition("background", func() Extension {
				done := make(chan struct{})
				return &callbacks{load: func(_ context.Context, h Host) error {
					go func() { defer close(done); <-h.Context().Done() }()
					return h.Command("ping", func(context.Context, Call) error { return nil })
				}, stop: func(ctx context.Context) error {
					select {
					case <-done:
						stopped++
						return nil
					case <-ctx.Done():
						return ctx.Err()
					}
				}}
			})
			m := testManager(t, d)
			if debug {
				if err := m.Debug(context.Background(), "background", events.Event{BotID: "a", Message: "/ping"}, nil); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := m.Enable(context.Background(), "background", "a"); err != nil {
					t.Fatal(err)
				}
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				if err := m.Disable(ctx, "background", "a"); err != nil {
					t.Fatal(err)
				}
			}
			if stopped != 1 {
				t.Fatal("background resource not joined", stopped)
			}
		})
	}
}

func TestDisableCancelsInflightInitialization(t *testing.T) {
	entered := make(chan struct{})
	cleaned := 0
	m := testManager(t, definition("initializing", func() Extension {
		return &callbacks{
			load: func(ctx context.Context, _ Host) error { close(entered); <-ctx.Done(); return ctx.Err() },
			stop: func(context.Context) error { cleaned++; return nil },
		}
	}))
	m.Timeout = 5 * time.Second
	enabled := make(chan error, 1)
	go func() { enabled <- m.Enable(context.Background(), "initializing", "a") }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("initialization not started")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := m.Disable(ctx, "initializing", "a"); err != nil {
		t.Fatal(err)
	}
	if err := <-enabled; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if cleaned != 1 || m.Config("initializing", "a").Enabled {
		t.Fatal("initializing instance leaked")
	}
}
