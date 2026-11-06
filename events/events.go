// Package events defines transport-neutral events and synchronous dispatch.
package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Type is an extensible canonical business event name.
type Type string

// Source identifies the conversation namespace.
type Source string

const (
	Group  Source = "group"
	C2C    Source = "c2c"
	Guild  Source = "guild"
	Direct Source = "direct"
)

// Reply is a transport-independent outgoing message capability request.
// Mention is an optional target user ID (member_openid for groups, user ID for
// guilds). Group text replies that mention a user are delivered as Markdown,
// because QQ only supports user mentions there in Markdown messages.
type Reply struct {
	Kind     string `json:"kind"`
	Text     string `json:"text,omitempty"`
	URL      string `json:"url,omitempty"`
	Keyboard string `json:"keyboard,omitempty"`
	Mention  string `json:"mention,omitempty"`
}

// ActionKind identifies a management capability request.
type ActionKind string

const (
	// ActionRecall removes a message in the current conversation.
	ActionRecall ActionKind = "recall"
	// ActionMute restricts one member for Action.Seconds.
	ActionMute ActionKind = "mute"
	// ActionUnmute clears a member restriction.
	ActionUnmute ActionKind = "unmute"
	// ActionKick removes one member from the current group or guild.
	ActionKick ActionKind = "kick"
)

// Action is a transport-independent management request. UserID is a
// member_openid in groups and a user ID in guilds. Seconds is used by mute.
type Action struct {
	Kind      ActionKind `json:"kind"`
	MessageID string     `json:"message_id,omitempty"`
	UserID    string     `json:"user_id,omitempty"`
	Seconds   int64      `json:"seconds,omitempty"`
	Blacklist bool       `json:"blacklist,omitempty"`
}

// Event is independent of SDK DTOs and legacy graph structures. Treat fields as immutable.
type Event struct {
	BotID                                        string
	Type                                         Type
	PlatformType, EventID, MessageID             string
	Source                                       Source
	UserID, GroupID, ChannelID, GuildID, Message string
	Raw                                          json.RawMessage
	PluginIDs                                    []string
	Reply                                        func(context.Context, Reply) error
	Act                                          func(context.Context, Action) error
}

// Handler processes one event. It must honor cancellation and synchronize shared state.
type Handler func(context.Context, Event) error

// Middleware wraps a handler for future policy, command routing or instrumentation.
type Middleware func(Handler) Handler

// Filter optionally scopes a subscription to a bot and event type.
type Filter struct {
	BotID string
	Type  Type
}
type subscription struct {
	id      uint64
	filter  Filter
	handler Handler
}

// Bus invokes matching handlers in subscription order. Publishers may run concurrently.
type Bus struct {
	mu       sync.RWMutex
	next     uint64
	handlers []subscription
	Timeout  time.Duration
}

func NewBus() *Bus { return &Bus{Timeout: 30 * time.Second} }

// Subscribe returns an idempotent removal function. In-flight calls may finish.
func (b *Bus) Subscribe(f Filter, h Handler) func() {
	b.mu.Lock()
	b.next++
	id := b.next
	b.handlers = append(b.handlers, subscription{id, f, h})
	b.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			b.mu.Lock()
			defer b.mu.Unlock()
			for i, s := range b.handlers {
				if s.id == id {
					b.handlers = append(b.handlers[:i], b.handlers[i+1:]...)
					break
				}
			}
		})
	}
}

// Publish isolates handler panics and errors, and continues with other subscribers.
func (b *Bus) Publish(ctx context.Context, e Event) error {
	b.mu.RLock()
	list := append([]subscription(nil), b.handlers...)
	b.mu.RUnlock()
	var errs []error
	for _, s := range list {
		if err := ctx.Err(); err != nil {
			return errors.Join(append(errs, err)...)
		}
		if s.filter.BotID != "" && s.filter.BotID != e.BotID || s.filter.Type != "" && s.filter.Type != e.Type {
			continue
		}
		hctx := ctx
		cancel := func() {}
		if b.Timeout > 0 {
			hctx, cancel = context.WithTimeout(ctx, b.Timeout)
		}
		err := invoke(hctx, s.handler, e)
		if err == nil {
			err = hctx.Err()
		}
		cancel()
		if err != nil {
			errs = append(errs, fmt.Errorf("handler %d: %w", s.id, err))
		}
	}
	return errors.Join(errs...)
}
func invoke(ctx context.Context, h Handler, e Event) (err error) {
	defer func() {
		if recover() != nil {
			err = errors.New("event handler panicked")
		}
	}()
	return h(ctx, e)
}
