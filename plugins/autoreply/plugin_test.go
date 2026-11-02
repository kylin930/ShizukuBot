package autoreply

import (
	"context"
	"shizukubot/events"
	"shizukubot/sdk"
	"testing"
)

func TestKeyword(t *testing.T) {
	for _, tc := range []struct {
		message, keyword string
		want             int
	}{{"hello world", "hello", 1}, {"world", "hello", 0}, {"world", "", 0}} {
		count := 0
		err := Respond(context.Background(), sdk.Call{Config: map[string]any{"keyword": tc.keyword, "reply": "answer"}, Event: events.Event{Message: tc.message, Reply: func(_ context.Context, r events.Reply) error {
			count++
			if r.Text != "answer" {
				t.Fatal(r)
			}
			return nil
		}}})
		if err != nil || count != tc.want {
			t.Fatal(tc, count, err)
		}
	}
}
