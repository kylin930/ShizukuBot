package rules

import (
	"context"
	"errors"
	"math"
	"shizukubot/events"
	"sync"
	"sync/atomic"
	"time"
)

type botState struct {
	write   sync.Mutex
	current atomic.Pointer[Set]
}

// Engine is one Bus subscriber with immutable per-Bot snapshots, not a plugin
// lifecycle manager. Configure Skip/OnError before Attach or concurrent use.
type Engine struct {
	Skip func(events.Event) bool
	// OnError receives only identifiers and error categories, never message text.
	OnError func(botID, ruleID, category string)
	Timeout time.Duration
	store   *Store
	states  sync.Map
	life    context.Context
	cancel  context.CancelFunc
	mu      sync.Mutex
	closed  bool
	off     func()
	workers sync.WaitGroup
	done    chan struct{}
}

// NewEngine owns store until Close; configure callbacks and Timeout before use.
func NewEngine(store *Store) *Engine {
	life, cancel := context.WithCancel(context.Background())
	return &Engine{store: store, life: life, cancel: cancel, Timeout: 5 * time.Second, done: make(chan struct{})}
}

// Attach subscribes exactly once. Dispatcher admission/dedup stays upstream.
func (e *Engine) Attach(bus *events.Bus) error {
	if bus == nil {
		return errors.New("event bus required")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return ErrClosed
	}
	if e.off != nil {
		return errors.New("rules engine already attached")
	}
	e.off = bus.Subscribe(events.Filter{}, func(ctx context.Context, event events.Event) error { _, err := e.Execute(ctx, event); return err })
	return nil
}
func (e *Engine) begin(ctx context.Context) (context.Context, func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		return nil, nil, ErrClosed
	}
	e.workers.Add(1)
	e.mu.Unlock()
	call, cancel := context.WithCancel(ctx)
	after := context.AfterFunc(e.life, cancel)
	return call, func() { after(); cancel(); e.workers.Done() }, nil
}
func (e *Engine) state(bot string) (*botState, error) {
	if !ValidScopeID(bot, 256) {
		return nil, errors.New("invalid Bot ID")
	}
	value, _ := e.states.LoadOrStore(bot, &botState{})
	return value.(*botState), nil
}
func (e *Engine) read(ctx context.Context, bot string) (*Set, error) {
	state, err := e.state(bot)
	if err != nil {
		return nil, err
	}
	if set := state.current.Load(); set != nil {
		return set, nil
	}
	state.write.Lock()
	defer state.write.Unlock()
	if set := state.current.Load(); set != nil {
		return set, nil
	}
	set, err := e.store.load(ctx, bot)
	if err != nil {
		return nil, err
	}
	state.current.Store(&set)
	return &set, nil
}

// Snapshot returns a copy; edits cannot mutate the live snapshot.
func (e *Engine) Snapshot(ctx context.Context, bot string) (Set, error) {
	ctx, finish, err := e.begin(ctx)
	if err != nil {
		return Set{}, err
	}
	defer finish()
	set, err := e.read(ctx, bot)
	if err != nil {
		return Set{}, err
	}
	return cloneSet(*set), nil
}

// Replace atomically commits an entire Bot set using a mandatory revision check.
// Existing events may finish their old snapshot; new reads see the committed one.
func (e *Engine) Replace(ctx context.Context, bot string, revision uint64, input []Rule) (Set, error) {
	ctx, finish, err := e.begin(ctx)
	if err != nil {
		return Set{}, err
	}
	defer finish()
	prepared, err := Prepare(input)
	if err != nil {
		return Set{}, err
	}
	if _, err = e.read(ctx, bot); err != nil {
		return Set{}, err
	}
	state, err := e.state(bot)
	if err != nil {
		return Set{}, err
	}
	state.write.Lock()
	defer state.write.Unlock()
	previous := state.current.Load()
	if revision != previous.Revision {
		return cloneSet(*previous), ErrConflict
	}
	if revision == math.MaxUint64 {
		return Set{}, errors.New("rule revision exhausted")
	}
	next := Set{SchemaVersion: SchemaVersion, BotID: bot, Revision: revision + 1, Rules: prepared}
	if err = e.store.commit(ctx, next); err != nil {
		return Set{}, err
	}
	state.current.Store(&next)
	return cloneSet(next), nil
}
func (e *Engine) run(ctx context.Context, set Set, event events.Event, preview bool) (Result, error) {
	skip := e.Skip != nil && e.Skip(event)
	send := event.Reply
	if preview {
		send = func(context.Context, events.Reply) error { return nil }
	}
	return evaluate(ctx, set, event, skip, send)
}

// Execute uses the event's existing reply capability with bounded cooperative time.
func (e *Engine) Execute(ctx context.Context, event events.Event) (Result, error) {
	ctx, finish, err := e.begin(ctx)
	if err != nil {
		return Result{}, err
	}
	defer finish()
	call, cancel := context.WithTimeout(ctx, e.Timeout)
	defer cancel()
	if !Eligible(event) {
		return Result{BotID: event.BotID, Eligible: false, SkipReason: "unsupported_event", Replies: []string{}, Traces: []Trace{}}, nil
	}
	set, err := e.read(call, event.BotID)
	if err != nil {
		if e.OnError != nil {
			e.OnError(event.BotID, "", "store_unavailable")
		}
		return Result{}, errors.New("rules unavailable for Bot")
	}
	out, err := e.run(call, *set, event, false)
	if e.OnError != nil {
		for _, trace := range out.Traces {
			if trace.Status == "send_failed" || trace.Status == "limited" {
				e.OnError(event.BotID, trace.RuleID, trace.Reason)
			}
		}
	}
	return out, err
}

// Preview evaluates saved rules (nil draft) or validated draft data with capture
// replies. It never publishes to Bus, calls plugins or touches production state.
func (e *Engine) Preview(ctx context.Context, event events.Event, draft []Rule) (Result, error) {
	ctx, finish, err := e.begin(ctx)
	if err != nil {
		return Result{}, err
	}
	defer finish()
	set, err := e.read(ctx, event.BotID)
	if err != nil {
		return Result{}, err
	}
	effective := *set
	if draft != nil {
		prepared, err := Prepare(draft)
		if err != nil {
			return Result{}, err
		}
		effective.Rules = prepared
	}
	call, cancel := context.WithTimeout(ctx, e.Timeout)
	defer cancel()
	out, err := e.run(call, effective, event, true)
	out.Draft = draft != nil
	return out, err
}

// Close cancels in-flight replies, removes its subscription and joins work.
// Cancellation is cooperative; no goroutine or sender is forcibly terminated.
func (e *Engine) Close(ctx context.Context) error {
	e.mu.Lock()
	if !e.closed {
		e.closed = true
		e.cancel()
		if e.off != nil {
			e.off()
			e.off = nil
		}
		go func() { e.workers.Wait(); close(e.done) }()
	}
	e.mu.Unlock()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-e.done:
		return e.store.Close()
	}
}
