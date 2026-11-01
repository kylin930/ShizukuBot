package example

import (
	"context"
	"encoding/json"
	"shizukubot/events"
	sdk "shizukubot/plugins"
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
	c := sdk.Call{Config: map[string]any{"prefix": "test"}, State: state, Event: events.Event{Reply: func(_ context.Context, r events.Reply) error { replies = append(replies, r.Text); return nil }}}
	for range 2 {
		if err := Count(context.Background(), c); err != nil {
			t.Fatal(err)
		}
	}
	if len(replies) != 2 || replies[0] != "test: 1" || replies[1] != "test: 2" {
		t.Fatal(replies)
	}
	state["count"] = json.RawMessage(`"corrupt"`)
	if Count(context.Background(), c) == nil {
		t.Fatal("corrupt state was silently reset")
	}
}
