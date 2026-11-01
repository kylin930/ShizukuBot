package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"shizukubot/events"
	"time"
)

// Debug uses a fresh instance and temporary state with caller-supplied capture-only replies.
func (m *Manager) Debug(ctx context.Context, id string, e events.Event, config map[string]any) (err error) {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return errors.New("plugin manager closed")
	}
	m.debugWG.Add(1)
	m.mu.Unlock()
	defer m.debugWG.Done()
	debugCtx, debugCancel := context.WithCancel(ctx)
	after := context.AfterFunc(m.life, debugCancel)
	defer after()
	defer debugCancel()
	ctx = debugCtx
	select {
	case m.debugSlots <- struct{}{}:
		defer func() { <-m.debugSlots }()
	case <-ctx.Done():
		return ctx.Err()
	}

	mf, f, err := m.manifest(id)
	if err != nil {
		return err
	}
	settings := m.Config(id, e.BotID)
	if !settings.Trusted {
		return errors.New("debug requires explicit trust")
	}
	if config == nil {
		config = settings.Config
	}
	cfg, err := mf.ConfigSchema.Validate(copyMap(config))
	if err != nil {
		return err
	}
	tmp, err := os.MkdirTemp("", "shizuku-debug-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	life, lifeCancel := context.WithCancel(ctx)
	defer lifeCancel()
	s := &slot{life: life, manifest: mf, status: Status{BotID: e.BotID, PluginID: id}, config: cfg.(map[string]any), state: &diskState{file: filepath.Join(tmp, "state.json")}}
	if err = m.start(ctx, s, f); err != nil {
		return err
	}
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		err = errors.Join(err, s.runtime.Stop(stopCtx))
	}()
	callCtx, cancel := context.WithTimeout(ctx, m.Timeout)
	defer cancel()
	call := Call{Event: e, Log: func(message string) {
		if mf.Allows("log.write") && m.Log != nil {
			m.Log(e.BotID, id, message)
		}
	}, Config: map[string]any{}, State: &permissionState{manifest: mf, inner: s.state}}
	if mf.Allows("config.read") {
		call.Config = copyMap(s.config)
	}
	if !mf.Allows("message.send") {
		call.Event.Reply = func(context.Context, events.Reply) error { return errors.New("message.send denied") }
	}
	return safe(func() error { return s.runtime.Handle(callCtx, call) })
}

type permissionState struct {
	manifest Manifest
	inner    State
}

func (p *permissionState) Get(ctx context.Context, k string) (json.RawMessage, error) {
	if !p.manifest.Allows("state.read") {
		return nil, errors.New("state.read denied")
	}
	return p.inner.Get(ctx, k)
}
func (p *permissionState) Set(ctx context.Context, k string, v json.RawMessage) error {
	if !p.manifest.Allows("state.write") {
		return errors.New("state.write denied")
	}
	return p.inner.Set(ctx, k, v)
}

// Packages includes installed JS projects and statically registered Go extensions.
