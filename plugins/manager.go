package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"shizukubot/events"
	"sort"
	"sync"
	"time"
)

// Settings are explicit per-Bot activation and user-controlled configuration.
type Settings struct {
	Enabled bool           `json:"enabled"`
	Trusted bool           `json:"trusted"`
	Config  map[string]any `json:"config"`
}
type Status struct {
	BotID     string `json:"bot_id"`
	PluginID  string `json:"plugin_id"`
	Enabled   bool   `json:"enabled"`
	State     string `json:"state"`
	LastError string `json:"last_error,omitempty"`
}
type slot struct {
	gate         chan struct{}
	mu           sync.Mutex
	status       Status
	manifest     Manifest
	runtime      Runtime
	registration Registration
	config       map[string]any
	state        State
	cancel       context.CancelFunc
	life         context.Context
	off          func()
	failures     int
	retry        time.Time
}

// Manager routes events through independent plugin/Bot instances. No process-global plugin state.
type Manager struct {
	// Skip is a pure host policy, set before enabling instances.
	Skip    func(events.Event) bool
	Store   *Store
	Bus     *events.Bus
	Node    string
	Timeout time.Duration
	Log     func(string, string, string)
	root    string
	mu      sync.RWMutex
	slots   map[string]*slot
	native  map[string]struct {
		m       Manifest
		factory Factory
	}
	settingsMu sync.Mutex
	settings   map[string]map[string]Settings
	closed     bool
	life       context.Context
	cancel     context.CancelFunc
	debugWG    sync.WaitGroup
	debugSlots chan struct{}
}

func NewManager(store *Store, bus *events.Bus, dataRoot string) (*Manager, error) {
	life, cancel := context.WithCancel(context.Background())
	m := &Manager{life: life, cancel: cancel, Store: store, Bus: bus, Timeout: 2 * time.Second, root: dataRoot, slots: map[string]*slot{}, settings: map[string]map[string]Settings{}, debugSlots: make(chan struct{}, 4), native: map[string]struct {
		m       Manifest
		factory Factory
	}{}}
	if err := os.MkdirAll(dataRoot, 0700); err != nil {
		return nil, err
	}
	b, err := os.ReadFile(filepath.Join(dataRoot, "settings.json"))
	if err == nil {
		err = json.Unmarshal(b, &m.settings)
	}
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return m, nil
}
func (m *Manager) getSlot(id, bot string) *slot {
	key := id + "\x00" + bot
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.slots[key]
	if s == nil {
		s = &slot{gate: make(chan struct{}, 1), status: Status{BotID: bot, PluginID: id, State: "unloaded"}}
		m.slots[key] = s
	}
	return s
}
func acquire(ctx context.Context, s *slot) error {
	select {
	case s.gate <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func release(s *slot) { <-s.gate }
func (m *Manager) Enable(ctx context.Context, id, bot string, trust bool) error {
	mf, factory, err := m.manifest(id)
	if err != nil {
		return err
	}
	settings := m.Config(id, bot)
	if !trust && !settings.Trusted {
		return errors.New("explicit trust required before executing plugin code")
	}
	cfg, err := mf.ConfigSchema.Validate(settings.Config)
	if err != nil {
		return err
	}
	s := m.getSlot(id, bot)
	if err = acquire(ctx, s); err != nil {
		return err
	}
	defer release(s)
	s.mu.Lock()
	if s.status.Enabled {
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()
	m.mu.RLock()
	closed := m.closed
	m.mu.RUnlock()
	if closed {
		return errors.New("plugin manager closed")
	}
	cctx, cancel := context.WithCancel(m.life)
	s.life = cctx
	s.mu.Lock()
	s.cancel = cancel
	s.mu.Unlock()
	s.manifest = mf
	s.config = cfg.(map[string]any)
	s.state = &diskState{file: filepath.Join(m.root, "state", id, scope(bot)+".json")}
	if err = m.start(ctx, s, factory); err != nil {
		cancel()
		return err
	}
	if err = m.saveSettings(id, bot, func(v *Settings) { v.Enabled = true; v.Trusted = true; v.Config = s.config }); err != nil {
		cancel()
		stopCtx, stopCancel := context.WithTimeout(context.Background(), time.Second)
		defer stopCancel()
		_ = s.runtime.Stop(stopCtx)
		s.runtime = nil
		return err
	}
	s.mu.Lock()
	s.status.Enabled = true
	s.off = m.Bus.Subscribe(events.Filter{BotID: bot}, func(ctx context.Context, e events.Event) error {
		if m.Skip != nil && m.Skip(e) {
			return nil
		}
		if len(e.PluginIDs) > 0 {
			found := false
			for _, id := range e.PluginIDs {
				if id == mf.ID {
					found = true
				}
			}
			if !found {
				return nil
			}
		}
		return m.dispatch(ctx, s, e, factory)
	})
	s.mu.Unlock()
	return nil
}
func (m *Manager) start(ctx context.Context, s *slot, f Factory) error {
	cfg := map[string]any{}
	if s.manifest.Allows("config.read") {
		cfg = copyMap(s.config)
	}
	var runtime Runtime
	err := safe(func() error { var err error; runtime, err = f(s.manifest, s.status.BotID, cfg, s.state); return err })
	if err == nil && runtime == nil {
		err = errors.New("factory returned no runtime")
	}
	if err != nil {
		return err
	}
	if native, ok := runtime.(*Native); ok {
		native.Life = s.life
	}
	startCtx, cancel := context.WithTimeout(ctx, m.Timeout)
	defer cancel()
	var reg Registration
	err = safe(func() error { var err error; reg, err = runtime.Start(startCtx); return err })
	if err == nil {
		if len(reg.Commands) > 128 || len(reg.Events) > 128 {
			err = errors.New("too many registrations")
		}
		if len(reg.Commands) > 0 && !s.manifest.Allows("commands.register") || len(reg.Events) > 0 && !s.manifest.Allows("events.subscribe") {
			err = errors.New("registration permission denied")
		}
		seen := map[string]bool{}
		for _, cmd := range reg.Commands {
			if !ValidID(cmd) || seen[cmd] {
				err = errors.New("invalid or duplicate command")
			}
			seen[cmd] = true
		}
	}
	if err != nil {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), time.Second)
		_ = runtime.Stop(stopCtx)
		stopCancel()
		m.failed(s, err)
		return err
	}
	s.runtime = runtime
	s.mu.Lock()
	s.registration = reg
	s.status.State = "running"
	s.status.LastError = ""
	s.mu.Unlock()
	return nil
}
func (m *Manager) failed(s *slot, err error) {
	s.failures++
	delay := time.Duration(s.failures) * 100 * time.Millisecond
	if delay > time.Second {
		delay = time.Second
	}
	s.retry = time.Now().Add(delay)
	s.mu.Lock()
	s.status.State = "failed"
	s.status.LastError = err.Error()
	if len(s.status.LastError) > 4096 {
		s.status.LastError = s.status.LastError[:4096]
	}
	s.mu.Unlock()
	if m.Log != nil {
		m.Log(s.status.BotID, s.status.PluginID, err.Error())
	}
}
func matches(reg Registration, e events.Event) bool {
	for _, t := range reg.Events {
		if t == "*" || t == e.Type {
			return true
		}
	}
	command, _, ok := ParseCommand(CommandText(e))
	if ok {
		for _, name := range reg.Commands {
			if name == command {
				return true
			}
		}
	}
	return false
}
func (m *Manager) dispatch(ctx context.Context, s *slot, e events.Event, f Factory) error {
	if err := acquire(ctx, s); err != nil {
		return err
	}
	defer release(s)
	if s.life.Err() != nil {
		return nil
	}
	callCtx, cancel := context.WithTimeout(ctx, m.Timeout)
	stop := context.AfterFunc(s.life, cancel)
	defer stop()
	defer cancel()
	if s.runtime == nil {
		if time.Now().Before(s.retry) {
			return errors.New("worker restart backoff")
		}
		if err := m.start(callCtx, s, f); err != nil {
			return err
		}
	}
	if !matches(s.registration, e) {
		return nil
	}
	c := Call{Event: e, Config: map[string]any{}, State: &permissionState{manifest: s.manifest, inner: s.state}}
	if s.manifest.Allows("config.read") {
		c.Config = copyMap(s.config)
	}
	original := e.Reply
	c.Event.Reply = func(ctx context.Context, r events.Reply) error {
		if !s.manifest.Allows("message.send") || original == nil {
			return errors.New("message.send denied")
		}
		return original(ctx, r)
	}
	c.Log = func(text string) {
		if s.manifest.Allows("log.write") && m.Log != nil {
			m.Log(e.BotID, s.manifest.ID, text)
		}
	}
	err := safe(func() error { return s.runtime.Handle(callCtx, c) })
	if err != nil {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), time.Second)
		_ = s.runtime.Stop(stopCtx)
		stopCancel()
		s.runtime = nil
		m.failed(s, err)
	} else {
		s.failures = 0
	}
	return err
}
func (m *Manager) Disable(ctx context.Context, id, bot string) error {
	s := m.getSlot(id, bot)
	s.mu.Lock()
	off, cancel := s.off, s.cancel
	if off != nil {
		off()
	}
	if cancel != nil {
		cancel()
	}
	s.mu.Unlock()
	if err := acquire(ctx, s); err != nil {
		return err
	}
	defer release(s)
	s.mu.Lock()
	if s.off != nil {
		s.off()
		s.off = nil
	}
	if s.cancel != nil {
		s.cancel()
	}
	s.mu.Unlock()
	var err error
	if s.runtime != nil {
		err = safe(func() error { return s.runtime.Stop(ctx) })
		s.runtime = nil
	}
	s.mu.Lock()
	s.status.Enabled = false
	s.status.State = "unloaded"
	s.mu.Unlock()
	saveErr := m.saveSettings(id, bot, func(v *Settings) { v.Enabled = false })
	return errors.Join(err, saveErr)
}
func (m *Manager) Restart(ctx context.Context, id, bot string) error {
	if err := m.Disable(ctx, id, bot); err != nil {
		return err
	}
	return m.Enable(ctx, id, bot, false)
}
func (m *Manager) Statuses() []Status {
	m.mu.RLock()
	list := make([]*slot, 0, len(m.slots))
	for _, s := range m.slots {
		list = append(list, s)
	}
	m.mu.RUnlock()
	out := []Status{}
	for _, s := range list {
		s.mu.Lock()
		out = append(out, s.status)
		s.mu.Unlock()
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PluginID+out[i].BotID < out[j].PluginID+out[j].BotID })
	return out
}

// Restore only starts explicitly trusted persisted instances for currently configured bots.
func (m *Manager) Restore(ctx context.Context, bots []string) error {
	var errs []error
	packages, err := m.Packages()
	if err != nil {
		return err
	}
	for _, p := range packages {
		for _, bot := range bots {
			s := m.Config(p.Manifest.ID, bot)
			if s.Enabled && s.Trusted {
				errs = append(errs, m.Enable(ctx, p.Manifest.ID, bot, false))
			}
		}
	}
	return errors.Join(errs...)
}
func (m *Manager) Close(ctx context.Context) error {
	m.mu.Lock()
	m.closed = true
	m.cancel()
	m.mu.Unlock()
	m.debugWG.Wait()
	var errs []error
	for _, s := range m.Statuses() {
		slot := m.getSlot(s.PluginID, s.BotID)
		slot.mu.Lock()
		if slot.off != nil {
			slot.off()
		}
		if slot.cancel != nil {
			slot.cancel()
		}
		slot.mu.Unlock()
		if err := acquire(ctx, slot); err != nil {
			errs = append(errs, err)
			continue
		}
		slot.mu.Lock()
		if slot.off != nil {
			slot.off()
			slot.off = nil
		}
		if slot.cancel != nil {
			slot.cancel()
		}
		slot.status.Enabled = false
		slot.status.State = "unloaded"
		slot.mu.Unlock()
		if slot.runtime != nil {
			errs = append(errs, slot.runtime.Stop(ctx))
			slot.runtime = nil
		}
		release(slot)
	}
	return errors.Join(errs...)
}
func (m *Manager) Commands(ids []string, bot string) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := []string{}
	for _, s := range m.slots {
		if s.status.BotID != bot {
			continue
		}
		s.mu.Lock()
		enabled := s.status.Enabled
		commands := append([]string(nil), s.registration.Commands...)
		s.mu.Unlock()
		if !enabled {
			continue
		}
		if len(ids) > 0 {
			found := false
			for _, id := range ids {
				if id == s.status.PluginID {
					found = true
				}
			}
			if !found {
				continue
			}
		}
		for _, cmd := range commands {
			out = append(out, "/"+cmd)
		}
	}
	sort.Strings(out)
	return out
}
