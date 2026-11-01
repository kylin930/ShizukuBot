package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"sort"
)

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
	if _, err := m.Store.Get(manifest.ID); err == nil {
		return errors.New("Go ID conflicts with installed code package")
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
		return &Worker{Node: m.Node, Entry: filepath.Join(p.Directory, mf.Entry), Manifest: mf, BotID: bot, Config: c, Log: func(message string) {
			if mf.Allows("log.write") && m.Log != nil {
				m.Log(bot, mf.ID, message)
			}
		}}, nil
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
	if _, err := json.Marshal(config); err != nil {
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
	m.mu.RLock()
	_, compiled := m.native[id]
	m.mu.RUnlock()
	if compiled {
		return errors.New("compiled extensions can only be disabled, not uninstalled")
	}

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
