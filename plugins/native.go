package plugins

import (
	"context"
	"errors"
	"fmt"
	"shizukubot/events"
	"sync"
	"unicode"
)

// Native adapts a fresh compiled Go extension instance; never reuse it across bots.
type Native struct {
	Extension    Extension
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
	if typ == "" || h == nil {
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
	if err != nil {
		_ = n.Stop(context.Background())
		return r, err
	}
	n.mu.Lock()
	n.sealed = true
	r = n.registration
	n.mu.Unlock()
	return r, nil
}
func (n *Native) Handle(ctx context.Context, c Call) error {
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
	return errors.Join(errs...)
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

// ParseCommand uses a literal slash prefix and shell-like quoted arguments, without expansion.
func ParseCommand(message string) (string, []string, bool) {
	fields := []string{}
	word := ""
	var quote rune
	escaped, started := false, false
	for _, r := range message {
		if escaped {
			word += string(r)
			escaped = false
			started = true
			continue
		}
		if r == '\\' {
			escaped = true
			started = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			} else {
				word += string(r)
			}
			started = true
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			started = true
			continue
		}
		if unicode.IsSpace(r) {
			if started {
				fields = append(fields, word)
				word = ""
				started = false
			}
			continue
		}
		word += string(r)
		started = true
	}
	if quote != 0 || escaped {
		return "", nil, false
	}
	if started {
		fields = append(fields, word)
	}
	if len(fields) == 0 || len(fields[0]) < 2 || fields[0][0] != '/' {
		return "", nil, false
	}
	name := fields[0][1:]
	if !ValidID(name) {
		return "", nil, false
	}
	return name, fields[1:], true
}

func (n *Native) Context() context.Context {
	if n.Life != nil {
		return n.Life
	}
	return context.Background()
}
