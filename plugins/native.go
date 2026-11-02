package plugins

import (
	"context"
	"errors"
	"fmt"
	"shizukubot/events"
	"shizukubot/sdk"
	"sync"
)

// Native adapts a fresh compiled Go extension instance; never reuse it across bots.
type Native struct {
	Extension    Extension
	bot          string
	config       map[string]any
	state        State
	log          func(string)
	Life         context.Context
	mu           sync.Mutex
	sealed       bool
	commands     map[string]func(context.Context, Call) error
	listeners    map[events.Type][]func(context.Context, Call) error
	registration Registration
	loaded       bool
}

func (n *Native) Command(name string, h func(context.Context, Call) error) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.sealed {
		return errors.New("registrations are sealed after Start")
	}
	if !ValidID(name) || h == nil {
		return errors.New("invalid command")
	}
	if _, ok := n.commands[name]; ok {
		return errors.New("duplicate command")
	}
	n.commands[name] = h
	n.registration.Commands = append(n.registration.Commands, name)
	return nil
}
func (n *Native) On(typ events.Type, h func(context.Context, Call) error) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.sealed {
		return errors.New("registrations are sealed after Start")
	}
	if !sdk.ValidEventType(typ) || h == nil {
		return errors.New("invalid subscription")
	}
	if len(n.listeners[typ]) == 0 {
		n.registration.Events = append(n.registration.Events, typ)
	}
	n.listeners[typ] = append(n.listeners[typ], h)
	return nil
}
func (n *Native) Start(ctx context.Context) (r Registration, err error) {
	n.mu.Lock()
	n.sealed = false
	n.commands = map[string]func(context.Context, Call) error{}
	n.listeners = map[events.Type][]func(context.Context, Call) error{}
	n.registration = Registration{}
	n.loaded = true
	n.mu.Unlock()
	if err = safe(func() error { return n.Extension.Load(ctx, n) }); err == nil {
		err = safe(func() error { return n.Extension.Start(ctx) })
	}
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		return r, err
	}
	n.mu.Lock()
	n.sealed = true
	r = n.registration
	n.mu.Unlock()
	return r, nil
}
func (n *Native) Handle(ctx context.Context, c Call) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	n.mu.Lock()
	commands := n.commands
	listeners := n.listeners
	n.mu.Unlock()
	var errs []error
	if command, args, ok := ParseCommand(CommandText(c.Event)); ok {
		if h := commands[command]; h != nil {
			c.Args = args
			errs = append(errs, safe(func() error { return h(ctx, c) }))
		}
	}
	for _, typ := range []events.Type{c.Event.Type, "*"} {
		for _, h := range listeners[typ] {
			errs = append(errs, safe(func() error { return h(ctx, c) }))
		}
	}
	errs = append(errs, safe(func() error { return n.Extension.HandleEvent(ctx, c) }))
	return errors.Join(append(errs, ctx.Err())...)
}
func (n *Native) Stop(ctx context.Context) error {
	n.mu.Lock()
	if !n.loaded {
		n.mu.Unlock()
		return nil
	}
	n.loaded = false
	n.sealed = true
	n.mu.Unlock()
	return errors.Join(safe(func() error { return n.Extension.Stop(ctx) }), safe(func() error { return n.Extension.Unload(ctx) }))
}
func safe(f func() error) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("plugin panic: %v", p)
		}
	}()
	return f()
}

func (n *Native) Context() context.Context {
	if n.Life != nil {
		return n.Life
	}
	return context.Background()
}

func (n *Native) BotID() string          { return n.bot }
func (n *Native) Config() map[string]any { return copyMap(n.config) }
func (n *Native) State() State           { return n.state }
func (n *Native) Log(message string) {
	if n.log != nil {
		n.log(message)
	}
}
