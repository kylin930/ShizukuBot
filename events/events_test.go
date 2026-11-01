package events

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestBusIsolationFailuresAndUnsubscribe(t *testing.T) {
	b := NewBus()
	var calls []string
	b.Subscribe(Filter{BotID: "a"}, func(context.Context, Event) error {
		calls = append(calls, "error")
		return errors.New("plugin failure")
	})
	b.Subscribe(Filter{BotID: "a"}, func(context.Context, Event) error { panic("plugin panic") })
	off := b.Subscribe(Filter{BotID: "a", Type: "c2c_message"}, func(context.Context, Event) error { calls = append(calls, "ok"); return nil })
	b.Subscribe(Filter{BotID: "b"}, func(context.Context, Event) error { t.Fatal("cross bot delivery"); return nil })
	if err := b.Publish(context.Background(), Event{BotID: "a", Type: "c2c_message"}); err == nil {
		t.Fatal("failures missing")
	}
	if len(calls) != 2 || calls[1] != "ok" {
		t.Fatalf("later handler skipped: %v", calls)
	}
	off()
	off()
	calls = nil
	b.Publish(context.Background(), Event{BotID: "a", Type: "c2c_message"})
	if len(calls) != 1 {
		t.Fatal(calls)
	}
}
func TestBusCancellationAndTimeout(t *testing.T) {
	b := NewBus()
	b.Timeout = time.Millisecond
	var after bool
	b.Subscribe(Filter{}, func(ctx context.Context, _ Event) error { <-ctx.Done(); return ctx.Err() })
	b.Subscribe(Filter{}, func(context.Context, Event) error { after = true; return nil })
	if !errors.Is(b.Publish(context.Background(), Event{}), context.DeadlineExceeded) || !after {
		t.Fatal("timeout prevented later handler")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if !errors.Is(b.Publish(ctx, Event{}), context.Canceled) {
		t.Fatal("cancel ignored")
	}
}
func TestIdentityDedupTTLAndCapacity(t *testing.T) {
	now := time.Now()
	d := NewDeduper(time.Second, 2)
	e := Event{BotID: "a", Type: "group_at_message", Source: Group, GroupID: "g", UserID: "u", MessageID: "one", Message: "same"}
	if d.seenAt(e, now) || !d.seenAt(e, now) {
		t.Fatal("duplicate delivery accepted")
	}
	e.Type = "group_message"
	if !d.seenAt(e, now) {
		t.Fatal("group aliases not collapsed")
	}
	e.MessageID = "two"
	if d.seenAt(e, now) {
		t.Fatal("legitimate identical text dropped")
	}
	e.BotID = "b"
	if d.seenAt(e, now) {
		t.Fatal("cross bot collision")
	}
	if len(d.entries) > 2 {
		t.Fatal("unbounded memory")
	}
	if d.seenAt(e, now.Add(2*time.Second)) {
		t.Fatal("TTL not applied")
	}
	e.MessageID = ""
	if d.Seen(e) || d.Seen(e) {
		t.Fatal("missing IDs must pass")
	}
	e.Type = "group_manage"
	e.EventID = "event"
	if d.Seen(e) || !d.Seen(e) {
		t.Fatal("event ID dedup broken")
	}
	e.PlatformType = "GROUP_DEL_ROBOT"
	if d.Seen(e) {
		t.Fatal("different management events collapsed")
	}
}
func TestConcurrentDispatch(t *testing.T) {
	b := NewBus()
	d := NewDispatcher(b)
	var count atomic.Int32
	b.Subscribe(Filter{}, func(context.Context, Event) error { count.Add(1); return nil })
	var wg sync.WaitGroup
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d.Dispatch(context.Background(), Event{BotID: "a", Type: "c2c_message", Source: C2C, MessageID: "one"})
		}()
	}
	wg.Wait()
	if count.Load() != 1 {
		t.Fatalf("executed %d times", count.Load())
	}
}

func TestMissingMessageIDFallsBackToExactEventID(t *testing.T) {
	d := NewDeduper(time.Minute, 10)
	e := Event{BotID: "a", Type: "c2c_message", Source: C2C, EventID: "event"}
	if d.Seen(e) || !d.Seen(e) {
		t.Fatal("event identity fallback failed")
	}
	e.Type = "group_message"
	if d.Seen(e) {
		t.Fatal("unknown alias must not collapse distinct event types")
	}
}
