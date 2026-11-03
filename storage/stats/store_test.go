package stats

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func mustOpen(t *testing.T, path string, o Options, now func() time.Time) *Store {
	t.Helper()
	s, e := open(path, o, nil, now)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := s.Close(); e != nil {
			t.Error(e)
		}
	})
	return s
}
func snap(t *testing.T, s *Store, ids ...string) Snapshot {
	t.Helper()
	v, e := s.Snapshot(context.Background(), ids)
	if e != nil {
		t.Fatal(e)
	}
	return v
}
func TestPersistenceIsolationDedupCalendarAndPrivacy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stats.db")
	var clock atomic.Int64
	loc, _ := time.LoadLocation("Asia/Shanghai")
	clock.Store(time.Date(2026, 10, 31, 23, 50, 0, 0, loc).Unix())
	now := func() time.Time { return time.Unix(clock.Load(), 0) }
	s := mustOpen(t, path, Defaults(), now)
	for _, bot := range []string{"a", "b"} {
		for i := 0; i < 2; i++ {
			for _, e := range []Event{{Bot: bot, Kind: "received"}, {Bot: bot, Kind: "sent"}, {Bot: bot, Kind: "user", Identity: "private-user-openid"}, {Bot: bot, Kind: "group", Identity: "private-group-openid"}} {
				if !s.Record(e) {
					t.Fatal("enqueue")
				}
			}
		}
	}
	v := snap(t, s, "a", "b")
	if v.UserCount != 2 || v.GroupCount != 2 || v.TodayDAU != 2 || v.SentMessages != 4 || v.Hourly[23].Received != 4 {
		t.Fatal(v)
	}
	if e := s.Close(); e != nil {
		t.Fatal(e)
	}
	s = mustOpen(t, path, Defaults(), now)
	s.Record(Event{Bot: "a", Kind: "user", Identity: "private-user-openid"})
	if v = snap(t, s, "a"); v.UserCount != 1 || v.TodayDAU != 1 || v.ReceivedMessages != 2 {
		t.Fatal(v)
	}
	clock.Add(3600)
	if v = snap(t, s, "a"); v.TodayDAU != 0 {
		t.Fatal(v)
	}
	s.Record(Event{Bot: "a", Kind: "received"})
	s.Record(Event{Bot: "a", Kind: "user", Identity: "private-user-openid"})
	if v = snap(t, s, "a"); v.UserCount != 1 || v.TodayDAU != 1 || v.Hourly[0].Received != 1 {
		t.Fatal(v)
	}
	history, e := s.History(context.Background(), []string{"a"}, 7)
	if e != nil || len(history) != 7 || history[5].Received != 2 || history[6].Received != 1 {
		t.Fatal(history, e)
	}
	// Removing a Bot excludes it from the total; reusing its stable ID resumes it.
	if snap(t, s, "b").SentMessages != 2 || snap(t, s, "a", "b").SentMessages != 4 {
		t.Fatal("bot ownership")
	}
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		b, _ := os.ReadFile(path + suffix)
		if strings.Contains(string(b), "private-user") || strings.Contains(string(b), "private-group") {
			t.Fatal("plaintext identity persisted")
		}
	}
	if stat, e := os.Stat(filepath.Join(filepath.Dir(path), ".stats-hmac-key")); e != nil || stat.Mode().Perm() != 0600 {
		t.Fatal(stat, e)
	}
}
func TestConcurrentWritesCloseFlushAndCleanup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stats.db")
	now := func() time.Time { return time.Date(2026, 11, 3, 4, 0, 0, 0, time.UTC) }
	o := Defaults()
	o.RetentionDays = 7
	s := mustOpen(t, path, o, now)
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				if !s.Record(Event{Bot: "a", Kind: "received"}) {
					t.Error("dropped")
				}
				s.Record(Event{Bot: "a", Kind: "user", Identity: "same"})
			}
		}()
	}
	wg.Wait()
	old := now().AddDate(0, 0, -7)
	s.Record(Event{Bot: "a", Kind: "sent", At: old})
	s.Record(Event{Bot: "a", Kind: "group", Identity: "old", At: old})
	if v := snap(t, s, "a"); v.ReceivedMessages != 800 || v.UserCount != 1 || v.SentMessages != 0 || v.GroupCount != 0 {
		t.Fatal(v)
	}
	for g := 0; g < 2; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 10; i++ {
				if err := s.Cleanup(context.Background()); err != nil {
					t.Error(err)
				}
				s.Record(Event{Bot: "a", Kind: "sent"})
			}
		}()
	}
	wg.Wait()
	if e := s.Close(); e != nil {
		t.Fatal(e)
	}
	s = mustOpen(t, path, o, now)
	if v := snap(t, s, "a"); v.SentMessages != 20 || v.ReceivedMessages != 800 {
		t.Fatal(v)
	}
	if len(snap(t, s, "a").Hourly) != 24 {
		t.Fatal("hour slots")
	}
	if s.Record(Event{Bot: "a", Kind: "user", Identity: "same"}) == false {
		t.Fatal("restart write")
	}
	if snap(t, s, "a").UserCount != 1 {
		t.Fatal("restart dedup")
	}
	o.RetentionDays = 0
	o.CleanupTime = "05:30"
	if e := s.Update(context.Background(), o); e != nil {
		t.Fatal(e)
	}
	s.Record(Event{Bot: "a", Kind: "sent", At: old})
	if e := s.Cleanup(context.Background()); e != nil {
		t.Fatal(e)
	}
	if snap(t, s, "a").SentMessages != 21 || s.Settings().CleanupTime != "05:30" {
		t.Fatal("permanent/update")
	}
}
func TestTransactionFailureRetryAndBoundedQueue(t *testing.T) {
	s := mustOpen(t, filepath.Join(t.TempDir(), "stats.db"), Defaults(), time.Now)
	if _, e := s.db.Exec("PRAGMA query_only=ON"); e != nil {
		t.Fatal(e)
	}
	s.Record(Event{Bot: "a", Kind: "received"})
	if e := s.Flush(context.Background()); e == nil {
		t.Fatal("read-only write silently succeeded")
	}
	if s.Status().State != "degraded" || s.Status().Error == "" {
		t.Fatal(s.Status())
	}
	// A failure must not publish a partial transaction or duplicate the retry.
	var n int
	if e := s.db.QueryRow("SELECT COUNT(*) FROM hourly").Scan(&n); e != nil || n != 0 {
		t.Fatal(n, e)
	}
	if _, e := s.db.Exec("PRAGMA query_only=OFF"); e != nil {
		t.Fatal(e)
	}
	if e := s.Flush(context.Background()); e != nil {
		t.Fatal(e)
	}
	if snap(t, s, "a").ReceivedMessages != 1 || s.Status().State != "ready" {
		t.Fatal(s.Status())
	}
	// An independent store without a running consumer proves the overflow contract.
	blocked := &Store{key: make([]byte, 32), queue: make(chan record, 1), loc: time.UTC, now: time.Now}
	if !blocked.Record(Event{Bot: "a", Kind: "sent"}) || blocked.Record(Event{Bot: "a", Kind: "sent"}) || blocked.Status().Dropped != 1 {
		t.Fatal("queue must be bounded and observable")
	}
}
func TestOpenFailuresAndSchema(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stats.db")
	if e := os.WriteFile(path, []byte("damaged sqlite"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := Open(path, Defaults(), nil); e == nil {
		t.Fatal("missing key for existing db")
	}
	os.WriteFile(filepath.Join(dir, ".stats-hmac-key"), make([]byte, 32), 0600)
	if _, e := Open(path, Defaults(), nil); e == nil {
		t.Fatal("corruption accepted")
	}
	if _, e := Open(filepath.Join(path, "bad"), Defaults(), nil); e == nil {
		t.Fatal("unwritable location accepted")
	}
	path = filepath.Join(t.TempDir(), "stats.db")
	s := mustOpen(t, path, Defaults(), time.Now)
	if _, e := s.db.Exec("PRAGMA user_version=99"); e != nil {
		t.Fatal(e)
	}
	s.Close()
	if _, e := Open(path, Defaults(), nil); e == nil {
		t.Fatal("future schema accepted")
	}
}
func TestScheduleCalendarDSTAndInvalidSettings(t *testing.T) {
	loc, _ := time.LoadLocation("America/New_York")
	for _, test := range []struct {
		stamp, at, last string
		want            bool
	}{
		{"2026-03-08T03:00:00-04:00", "02:30", "2026-03-07", true},
		{"2026-11-01T01:30:00-04:00", "01:30", "2026-10-31", true},
		{"2026-11-01T01:30:00-05:00", "01:30", "2026-11-01", false},
		{"2026-02-01T00:00:00-05:00", "00:00", "2026-01-31", true},
		{"2026-02-01T02:59:00-05:00", "03:00", "2026-01-31", false},
	} {
		now, _ := time.Parse(time.RFC3339, test.stamp)
		if got := Due(now.In(loc), test.at, test.last); got != test.want {
			t.Fatal(test, got)
		}
	}
	for _, o := range []Options{{8, "03:00", "UTC"}, {90, "3:00", "UTC"}, {90, "24:00", "UTC"}, {90, "03:00", "no-such-zone"}} {
		if o.Validate() == nil {
			t.Fatal(o)
		}
	}
}

func TestAtomicRollbackAndDailyScheduleUpdate(t *testing.T) {
	var clock atomic.Int64
	clock.Store(time.Date(2026, 3, 8, 3, 0, 0, 0, time.UTC).Unix())
	now := func() time.Time { return time.Unix(clock.Load(), 0) }
	o := Defaults()
	o.Timezone = "UTC"
	s := mustOpen(t, filepath.Join(t.TempDir(), "stats.db"), o, now)
	_, e := s.db.Exec(`CREATE TRIGGER reject_identity BEFORE INSERT ON identities BEGIN SELECT RAISE(FAIL,'test rollback'); END;`)
	if e != nil {
		t.Fatal(e)
	}
	s.Record(Event{Bot: "a", Kind: "received"})
	s.Record(Event{Bot: "a", Kind: "user", Identity: "u"})
	if e = s.Flush(context.Background()); e == nil {
		t.Fatal("transaction failure missing")
	}
	var total int
	s.db.QueryRow("SELECT COUNT(*) FROM hourly").Scan(&total)
	if total != 0 {
		t.Fatal("partial transaction committed")
	}
	s.db.Exec("DROP TRIGGER reject_identity")
	if e = s.Flush(context.Background()); e != nil {
		t.Fatal(e)
	}
	if v := snap(t, s, "a"); v.UserCount != 1 || v.ReceivedMessages != 1 {
		t.Fatal(v)
	}
	if e = s.call(context.Background(), func() error { s.schedule(); return nil }); e != nil {
		t.Fatal(e)
	}
	first := s.Status().LastCleanup
	clock.Add(60 * 60)
	o.CleanupTime = "04:00"
	s.Update(context.Background(), o)
	s.call(context.Background(), func() error { s.schedule(); return nil })
	if s.Status().LastCleanup != first {
		t.Fatal("schedule ran twice on same local date")
	}
	clock.Add(24 * 60 * 60)
	s.call(context.Background(), func() error { s.schedule(); return nil })
	if s.Status().LastCleanup == first {
		t.Fatal("updated next-day schedule did not run")
	}
}

func TestTimezoneAndReadOnlyDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stats.db")
	s := mustOpen(t, path, Defaults(), time.Now)
	s.Close()
	changed := Defaults()
	changed.Timezone = "UTC"
	if _, e := Open(path, changed, nil); e == nil {
		t.Fatal("mixed timezone accepted")
	}
	if e := os.Chmod(dir, 0500); e != nil {
		t.Fatal(e)
	}
	defer os.Chmod(dir, 0700)
	if _, e := Open(path, Defaults(), nil); e == nil {
		t.Fatal("read-only database location accepted")
	}
}
func BenchmarkBatch(b *testing.B) {
	s, e := Open(filepath.Join(b.TempDir(), "stats.db"), Defaults(), nil)
	if e != nil {
		b.Fatal(e)
	}
	defer s.Close()
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !s.Record(Event{Bot: "a", Kind: "received"}) || !s.Record(Event{Bot: "a", Kind: "user", Identity: "benchmark-user"}) {
			b.Fatal("unexpected drop")
		}
		if i%128 == 127 {
			if e = s.Flush(ctx); e != nil {
				b.Fatal(e)
			}
		}
	}
	if e = s.Flush(ctx); e != nil {
		b.Fatal(e)
	}
}
