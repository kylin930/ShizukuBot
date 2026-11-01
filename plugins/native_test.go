package plugins

import (
	"context"
	"shizukubot/events"
	"testing"
)

type extension struct{ start, stop, unload, calls int }

func (e *extension) Load(_ context.Context, h Host) error {
	return h.Command("ping", func(_ context.Context, c Call) error { e.calls++; return nil })
}
func (e *extension) Start(context.Context) error             { e.start++; return nil }
func (e *extension) HandleEvent(context.Context, Call) error { return nil }
func (e *extension) Stop(context.Context) error              { e.stop++; return nil }
func (e *extension) Unload(context.Context) error            { e.unload++; return nil }
func TestNativeLifecycle(t *testing.T) {
	e := &extension{}
	n := &Native{Extension: e}
	r, err := n.Start(context.Background())
	if err != nil || len(r.Commands) != 1 {
		t.Fatal(r, err)
	}
	if n.Command("ping", func(context.Context, Call) error { return nil }) == nil {
		t.Fatal("duplicate command accepted")
	}
	if err = n.Handle(context.Background(), Call{Event: events.Event{Message: "/ping"}}); err != nil {
		t.Fatal(err)
	}
	n.Stop(context.Background())
	n.Stop(context.Background())
	if e.start != 1 || e.calls != 1 || e.stop != 1 || e.unload != 1 {
		t.Fatal(e)
	}
}
func TestCommandArguments(t *testing.T) {
	name, args, ok := ParseCommand(`/echo "hello world" '' a\ b`)
	if !ok || name != "echo" || len(args) != 3 || args[0] != "hello world" || args[1] != "" || args[2] != "a b" {
		t.Fatal(name, args, ok)
	}
	if _, _, ok = ParseCommand(`/echo "unclosed`); ok {
		t.Fatal("malformed quote accepted")
	}
}
func TestConfigurationDefaults(t *testing.T) {
	s := Schema{Type: "object", Properties: map[string]Schema{"prefix": {Type: "string", Default: []byte(`"hi"`)}}, Required: []string{"prefix"}}
	v, err := s.Validate(nil)
	if err != nil || v.(map[string]any)["prefix"] != "hi" {
		t.Fatal(v, err)
	}
	if _, err = s.Validate(map[string]any{"secret": true}); err == nil {
		t.Fatal("unknown field accepted")
	}
}
