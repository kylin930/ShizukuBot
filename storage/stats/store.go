// Package stats stores only hourly counters and keyed identity digests.
package stats

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	_ "modernc.org/sqlite"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
	_ "time/tzdata"
)

const batchSize = 256
const queueSize = 4096

type Options struct {
	RetentionDays int    `json:"retention_days"`
	CleanupTime   string `json:"cleanup_time"`
	Timezone      string `json:"timezone"`
}

func Defaults() Options { return Options{90, "03:00", "Asia/Shanghai"} }
func (o Options) Validate() error {
	allowed := map[int]bool{0: true, 7: true, 30: true, 90: true, 180: true, 365: true}
	if !allowed[o.RetentionDays] {
		return errors.New("invalid statistics retention")
	}
	if t, err := time.Parse("15:04", o.CleanupTime); err != nil || t.Format("15:04") != o.CleanupTime {
		return errors.New("cleanup time must be HH:mm")
	}
	if _, err := time.LoadLocation(o.Timezone); err != nil || o.Timezone == "" {
		return errors.New("invalid statistics timezone")
	}
	return nil
}

type Event struct {
	Bot      string
	Kind     string
	Identity string
	At       time.Time
}
type record struct {
	bot, kind, day, digest string
	hour                   int
}
type Hour struct {
	Hour     string `json:"hour"`
	Sent     int64  `json:"sent"`
	Received int64  `json:"received"`
}
type Day struct {
	Date     string `json:"date"`
	Sent     int64  `json:"sent"`
	Received int64  `json:"received"`
}
type Snapshot struct {
	UserCount        int64  `json:"user_count"`
	GroupCount       int64  `json:"group_count"`
	TodayDAU         int64  `json:"today_dau"`
	SentMessages     int64  `json:"sent_messages"`
	ReceivedMessages int64  `json:"received_messages"`
	Hourly           []Hour `json:"hourly_stats"`
}
type Status struct {
	State       string  `json:"state"`
	Error       string  `json:"error,omitempty"`
	Dropped     uint64  `json:"dropped_events"`
	Pending     int     `json:"pending_events"`
	Bytes       int64   `json:"file_bytes"`
	LastCleanup string  `json:"last_cleanup"`
	Options     Options `json:"settings"`
}
type job struct {
	run     func() error
	done    chan error
	closing bool
}
type Store struct {
	db           *sql.DB
	path         string
	key          []byte
	mu           sync.RWMutex
	closed       bool
	opts         Options
	loc          *time.Location
	lastError    string
	dropped      uint64
	lastCleanup  string
	queue        chan record
	jobs         chan job
	done         chan struct{}
	pending      []record
	pendingCount atomic.Int64
	log          func(string)
	now          func() time.Time
}

func Open(path string, opts Options, log func(string)) (*Store, error) {
	return open(path, opts, log, time.Now)
}
func open(path string, opts Options, log func(string), now func() time.Time) (*Store, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, fmt.Errorf("statistics directory: %w", err)
	}
	keyPath := filepath.Join(filepath.Dir(path), ".stats-hmac-key")
	key, err := os.ReadFile(keyPath)
	if os.IsNotExist(err) {
		if st, e := os.Stat(path); e == nil && st.Size() > 0 {
			return nil, errors.New("statistics HMAC key missing; restore it with the database")
		}
		key = make([]byte, 32)
		if _, err = rand.Read(key); err != nil {
			return nil, err
		}
		f, e := os.OpenFile(keyPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e != nil {
			return nil, e
		}
		_, err = f.Write(key)
		if err == nil {
			err = f.Sync()
		}
		err = errors.Join(err, f.Close())
		if err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	if err = os.Chmod(keyPath, 0600); err != nil {
		return nil, err
	}
	if len(key) != 32 {
		return nil, errors.New("invalid statistics HMAC key")
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	fail := func(err error) (*Store, error) { db.Close(); return nil, fmt.Errorf("statistics database: %w", err) }
	for _, q := range []string{"PRAGMA busy_timeout=5000", "PRAGMA journal_mode=WAL", "PRAGMA synchronous=NORMAL", "PRAGMA foreign_keys=ON"} {
		if _, err = db.Exec(q); err != nil {
			return fail(err)
		}
	}
	var version int
	if err = db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return fail(err)
	}
	if version > 1 {
		return fail(errors.New("unsupported schema version"))
	}
	if version == 0 {
		tx, e := db.Begin()
		if e != nil {
			return fail(e)
		}
		_, e = tx.Exec(schema)
		if e != nil {
			tx.Rollback()
			return fail(e)
		}
		if e = tx.Commit(); e != nil {
			return fail(e)
		}
	}
	var storedZone string
	zoneErr := db.QueryRow("SELECT value FROM metadata WHERE key='timezone'").Scan(&storedZone)
	if errors.Is(zoneErr, sql.ErrNoRows) {
		if _, err = db.Exec("INSERT INTO metadata(key,value) VALUES('timezone',?)", opts.Timezone); err != nil {
			return fail(err)
		}
	} else if zoneErr != nil {
		return fail(zoneErr)
	} else if storedZone != opts.Timezone {
		return fail(errors.New("statistics timezone differs from database; restore the configured timezone"))
	}
	var check string
	if err = db.QueryRow("PRAGMA quick_check").Scan(&check); err != nil || check != "ok" {
		return fail(errors.Join(err, fmt.Errorf("integrity check: %s", check)))
	}
	if err = os.Chmod(path, 0600); err != nil {
		return fail(err)
	}
	loc, _ := time.LoadLocation(opts.Timezone)
	s := &Store{db: db, path: path, key: key, opts: opts, loc: loc, queue: make(chan record, queueSize), jobs: make(chan job), done: make(chan struct{}), log: log, now: now}
	if err = s.cleanup(); err != nil {
		return fail(err)
	}
	go s.loop()
	return s, nil
}

const schema = `CREATE TABLE hourly(bot TEXT NOT NULL, day TEXT NOT NULL, hour INTEGER NOT NULL CHECK(hour BETWEEN 0 AND 23), sent INTEGER NOT NULL DEFAULT 0, received INTEGER NOT NULL DEFAULT 0, PRIMARY KEY(bot,day,hour));
CREATE TABLE identities(bot TEXT NOT NULL, kind TEXT NOT NULL CHECK(kind IN ('user','group')), day TEXT NOT NULL, digest TEXT NOT NULL, PRIMARY KEY(bot,kind,day,digest));
CREATE INDEX hourly_day ON hourly(day); CREATE INDEX identities_day ON identities(day);
CREATE TABLE metadata(key TEXT PRIMARY KEY,value TEXT NOT NULL); PRAGMA user_version=1;`

func (s *Store) report(err error) {
	s.mu.Lock()
	previous := s.lastError
	if err == nil {
		s.lastError = ""
	} else {
		s.lastError = err.Error()
	}
	changed := previous != s.lastError
	s.mu.Unlock()
	if err != nil && changed && s.log != nil {
		s.log("statistics: " + err.Error())
	}
}

// Record never waits on SQLite or retains a plaintext user/group identifier.
func (s *Store) Record(e Event) bool {
	if e.Kind != "sent" && e.Kind != "received" && e.Kind != "user" && e.Kind != "group" {
		return false
	}
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return false
	}
	loc := s.loc
	at := e.At
	if at.IsZero() {
		at = s.now()
	}
	at = at.In(loc)
	r := record{bot: e.Bot, kind: e.Kind, day: at.Format("2006-01-02"), hour: at.Hour()}
	if e.Kind == "user" || e.Kind == "group" {
		if e.Identity == "" {
			s.mu.RUnlock()
			return true
		}
		mac := hmac.New(sha256.New, s.key)
		encoded, _ := json.Marshal([]string{e.Bot, e.Kind, e.Identity})
		mac.Write(encoded)
		r.digest = hex.EncodeToString(mac.Sum(nil))
	}
	select {
	case s.queue <- r:
		s.mu.RUnlock()
		return true
	default:
		s.mu.RUnlock()
		s.mu.Lock()
		s.dropped++
		s.mu.Unlock()
		s.report(errors.New("statistics queue full; event dropped"))
		return false
	}
}
func (s *Store) flush() error {
	if len(s.pending) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		s.report(err)
		return err
	}
	for _, r := range s.pending {
		if r.kind == "user" || r.kind == "group" {
			_, err = tx.Exec("INSERT OR IGNORE INTO identities(bot,kind,day,digest) VALUES(?,?,?,?)", r.bot, r.kind, r.day, r.digest)
		} else {
			sent, received := 0, 0
			if r.kind == "sent" {
				sent = 1
			} else {
				received = 1
			}
			_, err = tx.Exec("INSERT INTO hourly(bot,day,hour,sent,received) VALUES(?,?,?,?,?) ON CONFLICT(bot,day,hour) DO UPDATE SET sent=sent+excluded.sent,received=received+excluded.received", r.bot, r.day, r.hour, sent, received)
		}
		if err != nil {
			tx.Rollback()
			s.report(err)
			return err
		}
	}
	if err = tx.Commit(); err != nil {
		s.report(err)
		return err
	}
	s.pending = s.pending[:0]
	s.pendingCount.Store(0)
	s.report(nil)
	return nil
}
func (s *Store) drain() error {
	if err := s.flush(); err != nil {
		return err
	}
	for n := len(s.queue); n > 0; n-- {
		r := <-s.queue
		s.pending = append(s.pending, r)
		s.pendingCount.Store(int64(len(s.pending)))
		if len(s.pending) >= batchSize {
			if err := s.flush(); err != nil {
				return err
			}
		}
	}
	return s.flush()
}
func (s *Store) loop() {
	defer close(s.done)
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		var input <-chan record = s.queue
		if len(s.pending) >= batchSize {
			input = nil
		}
		select {
		case r := <-input:
			s.pending = append(s.pending, r)
			s.pendingCount.Store(int64(len(s.pending)))
			if len(s.pending) >= batchSize {
				_ = s.flush()
			}
		case j := <-s.jobs:
			err := s.drain()
			if err == nil && j.run != nil {
				err = j.run()
			}
			if err != nil {
				s.report(err)
			}
			if j.closing {
				err = errors.Join(err, s.db.Close())
				j.done <- err
				return
			}
			j.done <- err
		case <-tick.C:
			if err := s.flush(); err == nil {
				s.schedule()
			}
		}
	}
}
func (s *Store) call(ctx context.Context, run func() error) error {
	s.mu.RLock()
	closed := s.closed
	s.mu.RUnlock()
	if closed {
		return errors.New("statistics store closed")
	}
	j := job{run: run, done: make(chan error, 1)}
	select {
	case s.jobs <- j:
	case <-ctx.Done():
		return ctx.Err()
	case <-s.done:
		return errors.New("statistics store closed")
	}
	select {
	case err := <-j.done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (s *Store) Flush(ctx context.Context) error { return s.call(ctx, nil) }
func (s *Store) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		<-s.done
		return nil
	}
	s.closed = true
	s.mu.Unlock()
	j := job{done: make(chan error, 1), closing: true}
	s.jobs <- j
	err := <-j.done
	<-s.done
	return err
}
func (s *Store) Settings() Options { s.mu.RLock(); defer s.mu.RUnlock(); return s.opts }
func (s *Store) Update(ctx context.Context, o Options) error {
	if o.Timezone != s.Settings().Timezone {
		return errors.New("statistics timezone cannot change after database creation")
	}
	if err := o.Validate(); err != nil {
		return err
	}
	return s.call(ctx, func() error {
		s.mu.Lock()
		s.opts = o
		s.loc, _ = time.LoadLocation(o.Timezone)
		s.mu.Unlock()
		return nil
	})
}
func (s *Store) Status() Status {
	s.mu.RLock()
	v := Status{State: "ready", Error: s.lastError, Dropped: s.dropped, Pending: len(s.queue) + int(s.pendingCount.Load()), LastCleanup: s.lastCleanup, Options: s.opts}
	if s.closed {
		v.State = "closed"
	}
	s.mu.RUnlock()
	if v.Error != "" || v.Dropped > 0 {
		v.State = "degraded"
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if f, e := os.Stat(s.path + suffix); e == nil {
			v.Bytes += f.Size()
		}
	}
	return v
}
