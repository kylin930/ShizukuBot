package events

import (
	"context"
	"encoding/json"
	"sync"
	"time"
)

// Deduper stores bounded, expiring platform identities; zero IDs are never guessed.
type Deduper struct {
	mu          sync.Mutex
	entries     map[string]time.Time
	nextCleanup time.Time
	TTL         time.Duration
	Capacity    int
}

func NewDeduper(ttl time.Duration, capacity int) *Deduper {
	return &Deduper{entries: make(map[string]time.Time), TTL: ttl, Capacity: capacity}
}

// Identity aliases only the platform event types known to represent the same message.
func Identity(e Event) string {
	var parts []string
	switch e.Type {
	case "group_message", "group_at_message", "at_message", "guild_message", "c2c_message", "direct_message":
		if e.MessageID == "" {
			if e.EventID == "" {
				return ""
			}
			parts = []string{e.BotID, "event", string(e.Type), e.PlatformType, e.EventID}
			break
		}
		parts = []string{e.BotID, "message", string(e.Source), e.GroupID, e.ChannelID, e.GuildID, e.UserID, e.MessageID}
	default:
		if e.EventID == "" {
			return ""
		}
		parts = []string{e.BotID, "event", string(e.Type), e.PlatformType, e.EventID}
	}
	key, _ := json.Marshal(parts)
	return string(key)
}

// Seen atomically reserves an identity. Cleanup is lazy, without worker goroutines.
func (d *Deduper) Seen(e Event) bool { return d.seenAt(e, time.Now()) }
func (d *Deduper) seenAt(e Event, now time.Time) bool {
	key := Identity(e)
	if key == "" {
		return false
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if !now.Before(d.nextCleanup) {
		for k, t := range d.entries {
			if !t.After(now) {
				delete(d.entries, k)
			}
		}
		d.nextCleanup = now.Add(time.Second)
	}
	if until, ok := d.entries[key]; ok && until.After(now) {
		return true
	}
	if d.Capacity <= 0 || d.TTL <= 0 {
		return false
	}
	if len(d.entries) >= d.Capacity {
		var oldest string
		var t time.Time
		for k, v := range d.entries {
			if oldest == "" || v.Before(t) {
				oldest = k
				t = v
			}
		}
		delete(d.entries, oldest)
	}
	d.entries[key] = now.Add(d.TTL)
	return false
}

// Dispatcher owns message admission separately from subscriptions and adapters.
type Dispatcher struct {
	Bus   *Bus
	Dedup *Deduper
}

func NewDispatcher(bus *Bus) *Dispatcher { return &Dispatcher{bus, NewDeduper(5*time.Minute, 10000)} }
func (d *Dispatcher) Dispatch(ctx context.Context, e Event) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if d.Dedup.Seen(e) {
		return nil
	}
	return d.Bus.Publish(ctx, e)
}
