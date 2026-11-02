package sdk

import (
	"context"
	"errors"
	"sync"
)

// Option customizes a simple definition. Options only change its manifest;
// they never subscribe to an EventBus or execute the plugin.
type Option func(*Manifest)

// WithConfig declares a schema and explicitly grants configuration access.
func WithConfig(schema Schema) Option {
	return func(m *Manifest) { m.ConfigSchema = schema; addPermissions(m, "config.read") }
}

// WithPermissions adds explicit capabilities beyond the constructor's defaults.
// These are SDK capabilities, not operating-system security restrictions.
func WithPermissions(permissions ...string) Option {
	permissions = append([]string(nil), permissions...)
	return func(m *Manifest) { addPermissions(m, permissions...) }
}
func addPermissions(m *Manifest, permissions ...string) {
	for _, p := range permissions {
		if !m.Allows(p) {
			m.Permissions = append(m.Permissions, p)
		}
	}
}

// CommandPlugin defines one command with registration and reply permissions.
// It uses the current API version, Go runtime and an empty object schema.
func CommandPlugin(meta Meta, name string, handler CommandHandler, options ...Option) Definition {
	return simple(meta, []string{"commands.register", "message.send"}, func(r Registrar) error {
		return r.Command(name, handler)
	}, options)
}

// EventPlugin defines one callback for one or more canonical event types.
// The event list is copied; each instance registers through the normal Host.
func EventPlugin(meta Meta, types []EventType, handler EventHandler, options ...Option) Definition {
	types = append([]EventType(nil), types...)
	return simple(meta, []string{"events.subscribe", "message.send"}, func(r Registrar) error {
		if len(types) == 0 {
			return errors.New("at least one event type required")
		}
		for _, typ := range types {
			if err := r.On(typ, handler); err != nil {
				return err
			}
		}
		return nil
	}, options)
}

// Define provides lightweight initialization for multiple commands and events.
// register runs once for every new instance; allocate any instance-local handler
// state inside it. Defaults grant command/event registration and replies only.
func Define(meta Meta, register func(Registrar) error, options ...Option) Definition {
	return simple(meta, []string{"commands.register", "events.subscribe", "message.send"}, register, options)
}
func simple(meta Meta, permissions []string, register func(Registrar) error, options []Option) Definition {
	manifest := meta.manifest()
	manifest.Permissions = permissions
	for _, option := range options {
		if option != nil {
			option(&manifest)
		}
	}
	return Definition{Manifest: manifest, New: func() Extension { return &simplePlugin{register: register} }}
}

type simplePlugin struct {
	Base
	register func(Registrar) error
}

func (p *simplePlugin) Load(_ context.Context, h Host) (err error) {
	if p.register == nil {
		return errors.New("registration function required")
	}
	r := &registrar{host: h}
	// Close even when the callback panics; the native adapter recovers the panic.
	defer func() {
		registrationErr := r.close()
		if err == nil {
			err = registrationErr
		}
	}()
	return p.register(r)
}

type registrar struct {
	mu     sync.Mutex
	host   Host
	closed bool
	err    error
}

func (r *registrar) Command(name string, handler CommandHandler) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return errors.New("registrar is closed")
	}
	var err error
	if !ValidID(name) || handler == nil {
		err = errors.New("invalid command")
	} else {
		err = r.host.Command(name, handler)
	}
	if err != nil && r.err == nil {
		r.err = err
	}
	return err
}
func (r *registrar) On(typ EventType, handler EventHandler) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return errors.New("registrar is closed")
	}
	var err error
	if typ == "" || handler == nil {
		err = errors.New("invalid event subscription")
	} else {
		err = r.host.On(typ, handler)
	}
	if err != nil && r.err == nil {
		r.err = err
	}
	return err
}
func (r *registrar) close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
	r.host = nil
	return r.err
}
