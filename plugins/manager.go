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
	debugSlots chan struct{}
}

func NewManager(store *Store, bus *events.Bus, dataRoot string) (*Manager, error) {
	m := &Manager{Store: store, Bus: bus, Timeout: 2 * time.Second, root: dataRoot, slots: map[string]*slot{}, settings: map[string]map[string]Settings{}, debugSlots: make(chan struct{}, 4), native: map[string]struct {
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
func (m *Manager) RegisterGo(manifest Manifest, create func() Extension) error {
	if err := manifest.Validate(); err != nil {
		return err
	}
	if manifest.Runtime != "go" || create == nil {
		return errors.New("invalid Go registration")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.native[manifest.ID]; ok {
		return errors.New("duplicate Go registration")
	}
	m.native[manifest.ID] = struct {
		m       Manifest
		factory Factory
	}{manifest, func(Manifest, string, map[string]any, State) (Runtime, error) {
		return &Native{Extension: create()}, nil
	}}
	return nil
}
func (m *Manager) manifest(id string) (Manifest, Factory, error) {
	m.mu.RLock()
	native, ok := m.native[id]
	m.mu.RUnlock()
	if ok {
		return native.m, native.factory, nil
	}
	p, err := m.Store.Get(id)
	if err != nil {
		return Manifest{}, nil, err
	}
	return p.Manifest, func(mf Manifest, bot string, c map[string]any, s State) (Runtime, error) {
		return &Worker{Node: m.Node, Entry: filepath.Join(p.Directory, mf.Entry), Manifest: mf, BotID: bot, Config: c}, nil
	}, nil
}
func (m *Manager) Config(id, bot string) Settings {
	m.settingsMu.Lock()
	defer m.settingsMu.Unlock()
	s := m.settings[id][bot]
	s.Config = copyMap(s.Config)
	return s
}
func copyMap(v map[string]any) map[string]any {
	data, _ := json.Marshal(v)
	var out map[string]any
	_ = json.Unmarshal(data, &out)
	if out == nil {
		out = map[string]any{}
	}
	return out
}
func (m *Manager) Configure(id, bot string, config map[string]any) error {
	mf, _, err := m.manifest(id)
	if err != nil {
		return err
	}
	v, err := mf.ConfigSchema.Validate(copyMap(config))
	if err != nil {
		return err
	}
	return m.saveSettings(id, bot, func(s *Settings) { s.Config = v.(map[string]any) })
}
func (m *Manager) saveSettings(id, bot string, update func(*Settings)) error {
	if bot == "" || len(bot) > 256 {
		return errors.New("invalid bot id")
	}
	m.settingsMu.Lock()
	defer m.settingsMu.Unlock()
	all := map[string]map[string]Settings{}
	b, err := json.Marshal(m.settings)
	if err != nil {
		return err
	}
	if err = json.Unmarshal(b, &all); err != nil {
		return err
	}
	if all[id] == nil {
		all[id] = map[string]Settings{}
	}
	s := all[id][bot]
	update(&s)
	all[id][bot] = s
	if err = AtomicJSON(filepath.Join(m.root, "settings.json"), all); err != nil {
		return err
	}
	m.settings = all
	return nil
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
	cctx, cancel := context.WithCancel(context.Background())
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
	runtime, err := f(s.manifest, s.status.BotID, cfg, s.state)
	if err != nil {
		return err
	}
	startCtx, cancel := context.WithTimeout(ctx, m.Timeout)
	defer cancel()
	var reg Registration
	err = safe(func() error { var err error; reg, err = runtime.Start(startCtx); return err })
	if err == nil {
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
	m.mu.Unlock()
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

// Debug uses a fresh instance and temporary state; production subscriptions and senders are not involved.
func (m *Manager) Debug(ctx context.Context, id string, e events.Event, config map[string]any) (err error) {
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
	s := &slot{manifest: mf, status: Status{BotID: e.BotID, PluginID: id}, config: cfg.(map[string]any), state: &diskState{file: filepath.Join(tmp, "state.json")}}
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
	call := Call{Event: e, Config: map[string]any{}, State: &permissionState{manifest: mf, inner: s.state}}
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
func (m *Manager) Packages() ([]Package, error) {
	out, err := m.Store.List()
	if err != nil {
		return nil, err
	}
	m.mu.RLock()
	for _, n := range m.native {
		out = append(out, Package{Manifest: n.m})
	}
	m.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Manifest.ID < out[j].Manifest.ID })
	return out, nil
}
func (m *Manager) Trust(id, bot string, trusted bool) error {
	if _, _, err := m.manifest(id); err != nil {
		return err
	}
	return m.saveSettings(id, bot, func(s *Settings) { s.Trusted = trusted })
}
func (m *Manager) Remove(ctx context.Context, id string) error {
	for _, s := range m.Statuses() {
		if s.PluginID == id {
			if err := m.Disable(ctx, id, s.BotID); err != nil {
				return err
			}
		}
	}
	m.settingsMu.Lock()
	for bot, s := range m.settings[id] {
		s.Trusted = false
		s.Enabled = false
		m.settings[id][bot] = s
	}
	err := AtomicJSON(filepath.Join(m.root, "settings.json"), m.settings)
	m.settingsMu.Unlock()
	if err != nil {
		return err
	}
	return m.Store.Remove(id)
}
