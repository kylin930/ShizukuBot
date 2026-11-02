package example

import (
	"context"
	"encoding/json"
	"shizukubot/events"
	"shizukubot/sdk"
	"testing"
)

type memoryState map[string]json.RawMessage

func (s memoryState) Get(_ context.Context, k string) (json.RawMessage, error) {
	return append(json.RawMessage(nil), s[k]...), nil
}
func (s memoryState) Set(_ context.Context, k string, v json.RawMessage) error {
	s[k] = append(json.RawMessage(nil), v...)
	return nil
}
func TestCounter(t *testing.T) {
	state := memoryState{}
	replies := []string{}
	p := &Plugin{}
	host := &testHost{state: state, config: map[string]any{"prefix": "test"}}
	if err := p.Load(context.Background(), host); err != nil {
		t.Fatal(err)
	}
	if err := p.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	c := sdk.Call{Config: map[string]any{"prefix": "test"}, State: state, Event: events.Event{Reply: func(_ context.Context, r events.Reply) error { replies = append(replies, r.Text); return nil }}}
	for range 2 {
		if err := p.Count(context.Background(), c); err != nil {
			t.Fatal(err)
		}
	}
	if len(replies) != 2 || replies[0] != "test: 1" || replies[1] != "test: 2" {
		t.Fatal(replies)
	}
	state["count"] = json.RawMessage(`"corrupt"`)
	if err := p.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := p.Unload(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := p.Load(context.Background(), host); err != nil {
		t.Fatal(err)
	}
	if p.Start(context.Background()) == nil {
		t.Fatal("corrupt state was silently reset")
	}
}

type testHost struct {
	state  sdk.State
	config map[string]any
}

func (h *testHost) Context() context.Context                 { return context.Background() }
func (h *testHost) BotID() string                            { return "test" }
func (h *testHost) Config() map[string]any                   { return h.config }
func (h *testHost) State() sdk.State                         { return h.state }
func (h *testHost) Log(string)                               {}
func (h *testHost) Command(string, sdk.CommandHandler) error { return nil }
func (h *testHost) On(sdk.EventType, sdk.EventHandler) error { return nil }

type unreliableState struct {
	memoryState
	fail bool
}

func (s *unreliableState) Set(ctx context.Context, k string, v json.RawMessage) error {
	if s.fail {
		return context.DeadlineExceeded
	}
	return s.memoryState.Set(ctx, k, v)
}
func TestCounterWriteFailureAndLifecycleRestart(t *testing.T) {
	state := &unreliableState{memoryState: memoryState{}}
	host := &testHost{state: state, config: map[string]any{"prefix": "test"}}
	instance := &Plugin{}
	start := func() {
		t.Helper()
		if err := instance.Load(context.Background(), host); err != nil {
			t.Fatal(err)
		}
		if err := instance.Start(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	start()
	replies := []string{}
	c := sdk.Call{State: state, Event: sdk.Event{Reply: func(_ context.Context, r sdk.Reply) error { replies = append(replies, r.Text); return nil }}}
	if err := instance.Count(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	state.fail = true
	if err := instance.Count(context.Background(), c); err == nil {
		t.Fatal("write error hidden")
	}
	state.fail = false
	if err := instance.Count(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if len(replies) != 2 || replies[1] != "test: 2" {
		t.Fatal("failed write advanced cached counter", replies)
	}
	if err := instance.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := instance.Unload(context.Background()); err != nil {
		t.Fatal(err)
	}
	if instance.host != nil || instance.ready {
		t.Fatal("host references retained")
	}
	if instance.Count(context.Background(), c) == nil {
		t.Fatal("stopped instance still active")
	}
	instance = &Plugin{}
	start()
	if err := instance.Count(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if len(replies) != 3 || replies[2] != "test: 3" {
		t.Fatal("restart lost persisted counter", replies)
	}
}
