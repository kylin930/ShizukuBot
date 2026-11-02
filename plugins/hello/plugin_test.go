package hello

import (
	"context"
	"shizukubot/events"
	"shizukubot/sdk"
	"testing"
)

func TestPing(t *testing.T) {
	reply := ""
	err := Ping(context.Background(), sdk.Call{Event: events.Event{Reply: func(_ context.Context, r events.Reply) error { reply = r.Text; return nil }}})
	if err != nil || reply != "pong" {
		t.Fatal(reply, err)
	}
	if err = Ping(context.Background(), sdk.Call{}); err == nil {
		t.Fatal("missing reply capability ignored")
	}
}
