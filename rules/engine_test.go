package rules

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"shizukubot/events"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func testEngine(t *testing.T) (*Engine, string) {
	t.Helper()
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	engine := NewEngine(store)
	t.Cleanup(func() {
		if err := engine.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})
	return engine, dir
}
func put(t *testing.T, e *Engine, bot string, revision uint64, rs ...Rule) Set {
	t.Helper()
	set, err := e.Replace(context.Background(), bot, revision, rs)
	if err != nil {
		t.Fatal(err)
	}
	return set
}
func TestOnlineSnapshotsBotIsolationAndRestart(t *testing.T) {
	e, dir := testEngine(t)
	ctx := context.Background()
	a := sample("same")
	a.Action.Text = "A"
	b := a
	b.Action.Text = "B"
	saved := put(t, e, "a", 0, a)
	put(t, e, "b", 0, b)
	saved.Rules[0].Action.Text = "mutated"
	saved.Rules[0].Conditions.Sources[0] = events.Guild
	event := message()
	event.Reply = func(_ context.Context, r events.Reply) error {
		if r.Text != "A" {
			t.Errorf("mutable snapshot: %s", r.Text)
		}
		return nil
	}
	if out, err := e.Execute(ctx, event); err != nil || len(out.Replies) != 1 {
		t.Fatal(out, err)
	}
	a.Enabled = false
	put(t, e, "a", 1, a)
	if out, err := e.Execute(ctx, event); err != nil || len(out.Replies) != 0 {
		t.Fatal(out, err)
	}
	event.BotID = "b"
	event.Reply = func(_ context.Context, r events.Reply) error {
		if r.Text != "B" {
			t.Error(r)
		}
		return nil
	}
	if out, err := e.Execute(ctx, event); err != nil || len(out.Replies) != 1 {
		t.Fatal(out, err)
	}
	put(t, e, "a", 2)
	if err := e.Close(ctx); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	restarted := NewEngine(store)
	defer restarted.Close(ctx)
	for _, bot := range []string{"a", "b"} {
		set, err := restarted.Snapshot(ctx, bot)
		if err != nil {
			t.Fatal(err)
		}
		if bot == "a" && (len(set.Rules) != 0 || set.Revision != 3) || bot == "b" && (len(set.Rules) != 1 || set.Rules[0].Action.Text != "B") {
			t.Fatal(set)
		}
	}
}
func TestConcurrentAdminRevisionAndEventSnapshots(t *testing.T) {
	e, _ := testEngine(t)
	ctx := context.Background()
	put(t, e, "a", 0, sample("r"))
	entered, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	event := message()
	event.Reply = func(ctx context.Context, r events.Reply) error {
		close(entered)
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	go func() { _, err := e.Execute(ctx, event); done <- err }()
	<-entered
	var wg sync.WaitGroup
	var won, conflict atomic.Int32
	for i := 0; i < 2; i++ {
		wg.Go(func() {
			_, err := e.Replace(ctx, "a", 1, []Rule{})
			if err == nil {
				won.Add(1)
			} else if errors.Is(err, ErrConflict) {
				conflict.Add(1)
			} else {
				t.Error(err)
			}
		})
	}
	// A blocked sender holds no snapshot/store/global lock.
	updated := make(chan struct{})
	go func() { wg.Wait(); close(updated) }()
	select {
	case <-updated:
	case <-time.After(2 * time.Second):
		close(release)
		t.Fatal("save blocked by reply")
	}
	if won.Load() != 1 || conflict.Load() != 1 {
		t.Fatal(won.Load(), conflict.Load())
	}
	put(t, e, "b", 0, sample("r"))
	if out, err := e.Preview(ctx, message(), nil); err != nil || len(out.Replies) != 0 {
		t.Fatal(out, err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	// Concurrent readers and online replacements exercise race-sensitive slice ownership.
	for i := 0; i < 8; i++ {
		wg.Go(func() {
			for j := 0; j < 25; j++ {
				if _, err := e.Preview(ctx, message(), nil); err != nil {
					t.Error(err)
				}
			}
		})
	}
	for rev := uint64(2); rev < 12; rev++ {
		put(t, e, "a", rev, sample("r"))
	}
	wg.Wait()
}
func TestFailedCommitPreservesFileAndMemory(t *testing.T) {
	e, dir := testEngine(t)
	put(t, e, "a", 0, sample("old"))
	path := filepath.Join(dir, filename("a"))
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Inject a real filesystem failure without permission assumptions (tests may run as root).
	if err = e.store.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = e.Replace(context.Background(), "a", 1, []Rule{sample("new")}); err == nil {
		t.Fatal("closed filesystem accepted write")
	}
	after, err := os.ReadFile(path)
	if err != nil || string(before) != string(after) {
		t.Fatal("old file changed", err)
	}
	set, err := e.Snapshot(context.Background(), "a")
	if err != nil || set.Revision != 1 || set.Rules[0].ID != "old" {
		t.Fatal(set, err)
	}
}
func TestCorruptVersionAndNonregularFilesPreserved(t *testing.T) {
	for _, raw := range []string{`broken`, `{"schema_version":99,"bot_id":"a","rules":[]}`, `{"schema_version":1,"bot_id":"b","rules":[]}`, `{"schema_version":1,"bot_id":"a","rules":[]} {}`} {
		t.Run(raw, func(t *testing.T) {
			e, dir := testEngine(t)
			path := filepath.Join(dir, filename("a"))
			if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := e.Replace(context.Background(), "a", 0, []Rule{}); err == nil {
				t.Fatal("invalid file overwritten")
			}
			after, err := os.ReadFile(path)
			if err != nil || string(after) != raw {
				t.Fatal(string(after), err)
			}
		})
	}
	e, dir := testEngine(t)
	path := filepath.Join(dir, filename("a"))
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Snapshot(context.Background(), "a"); err == nil {
		t.Fatal("directory accepted")
	}
	// Bot IDs are opaque data, never path segments.
	put(t, e, "../opaque", 0, sample("r"))
	if _, err := os.Stat(filepath.Join(dir, filename("../opaque"))); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Snapshot(context.Background(), "bad id"); err == nil {
		t.Fatal("invalid Bot ID")
	}
}
func TestPreviewCaptureAndBuiltinSkip(t *testing.T) {
	e, dir := testEngine(t)
	ctx := context.Background()
	put(t, e, "a", 0, sample("r"))
	before, _ := os.ReadFile(filepath.Join(dir, filename("a")))
	event := message()
	event.Reply = func(context.Context, events.Reply) error { t.Fatal("preview reached real sender"); return nil }
	draft := sample("draft")
	draft.Action.Text = "draft"
	out, err := e.Preview(ctx, event, []Rule{draft})
	if err != nil || !out.Draft || len(out.Replies) != 1 || out.Replies[0] != "draft" {
		t.Fatal(out, err)
	}
	after, _ := os.ReadFile(filepath.Join(dir, filename("a")))
	if string(before) != string(after) {
		t.Fatal("preview wrote production data")
	}
	e.Skip = func(events.Event) bool { return true }
	out, err = e.Preview(ctx, event, nil)
	if err != nil || len(out.Replies) != 0 || out.Traces[0].Status != "skipped_builtin" {
		t.Fatal(out, err)
	}
}
func TestDispatcherDedupAndCloseCancellation(t *testing.T) {
	e, _ := testEngine(t)
	put(t, e, "a", 0, sample("r"))
	bus := events.NewBus()
	if err := e.Attach(bus); err != nil {
		t.Fatal(err)
	}
	if e.Attach(bus) == nil {
		t.Fatal("double subscription")
	}
	d := events.NewDispatcher(bus)
	event := message()
	event.MessageID = "one"
	var calls int
	event.Reply = func(context.Context, events.Reply) error { calls++; return nil }
	for _, typ := range []events.Type{"group_message", "group_at_message"} {
		event.Type = typ
		if err := d.Dispatch(context.Background(), event); err != nil {
			t.Fatal(err)
		}
	}
	event.MessageID = "two"
	if err := d.Dispatch(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatal(calls)
	}
	started, done := make(chan struct{}), make(chan error, 1)
	event.Reply = func(ctx context.Context, _ events.Reply) error { close(started); <-ctx.Done(); return ctx.Err() }
	go func() { _, err := e.Execute(context.Background(), event); done <- err }()
	<-started
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := e.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := e.Snapshot(ctx, "a"); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
	if err := bus.Publish(ctx, event); err != nil {
		t.Fatal("subscription leaked", err)
	}
}
func TestPanickingSenderAndTimeoutAreIsolated(t *testing.T) {
	e, _ := testEngine(t)
	put(t, e, "a", 0, sample("a"), sample("b"))
	event := message()
	sent := 0
	event.Reply = func(context.Context, events.Reply) error {
		sent++
		if sent == 1 {
			panic("secret SDK internals")
		}
		return nil
	}
	out, err := e.Execute(context.Background(), event)
	if !errors.Is(err, ErrReplyFailed) || strings.Contains(err.Error(), "secret") || len(out.Replies) != 1 {
		t.Fatal(out, err)
	}
	e.Timeout = 10 * time.Millisecond
	event.Reply = func(ctx context.Context, _ events.Reply) error { <-ctx.Done(); return ctx.Err() }
	out, err = e.Execute(context.Background(), event)
	if !errors.Is(err, context.DeadlineExceeded) || out.Attempts != 1 || out.Traces[0].Reason != "timeout" {
		t.Fatal(out, err)
	}
}
