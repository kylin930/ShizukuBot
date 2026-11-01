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
	s := &slot{cancel: lifeCancel, life: life, manifest: mf, status: Status{BotID: e.BotID, PluginID: id}, config: cfg.(map[string]any), state: &diskState{file: filepath.Join(tmp, "state.json")}}
	if err = m.start(ctx, s, f); err != nil {
		return err
	}
	defer func() {
		lifeCancel()
		stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		err = errors.Join(err, s.runtime.Stop(stopCtx))
	}()
	if !matches(s.registration, e) {
		return nil
	}
	callCtx, cancel := context.WithTimeout(ctx, m.Timeout)
	defer cancel()
	call := m.call(s, e)
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
