package plugins

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"sort"
	"sync"
)

// Registry is a build-time catalog, without Bot state or a dynamic code loader.
type Registry struct {
	mu          sync.RWMutex
	definitions map[string]Definition
}

func NewRegistry() *Registry { return &Registry{definitions: map[string]Definition{}} }

// Register rejects incompatible definitions and duplicate IDs.
func (r *Registry) Register(d Definition) error {
	if err := d.Validate(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.definitions == nil {
		r.definitions = map[string]Definition{}
	}
	if _, ok := r.definitions[d.Manifest.ID]; ok {
		return errors.New("duplicate plugin ID: " + d.Manifest.ID)
	}
	d.Manifest = cloneManifest(d.Manifest)
	r.definitions[d.Manifest.ID] = d
	return nil
}
func cloneManifest(m Manifest) Manifest {
	b, _ := json.Marshal(m)
	var out Manifest
	_ = json.Unmarshal(b, &out)
	return out
}
func (r *Registry) snapshot() map[string]Definition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := map[string]Definition{}
	for id, d := range r.definitions {
		d.Manifest = cloneManifest(d.Manifest)
		out[id] = d
	}
	return out
}
func (m *Manager) manifest(id string) (Manifest, func() Extension, error) {
	d, ok := m.registry[id]
	if !ok {
		return Manifest{}, nil, errors.New("plugin is not compiled into this binary")
	}
	return cloneManifest(d.Manifest), d.New, nil
}

// Package is catalog metadata, not an installable source archive.
type Package struct {
	Manifest Manifest `json:"manifest"`
}

func (m *Manager) Packages() ([]Package, error) {
	out := []Package{}
	for _, d := range m.registry {
		out = append(out, Package{Manifest: cloneManifest(d.Manifest)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Manifest.ID < out[j].Manifest.ID })
	return out, nil
}
func (m *Manager) Config(id, bot string) Settings {
	m.settingsMu.Lock()
	defer m.settingsMu.Unlock()
	s := m.settings[id][bot]
	s.Config = copyMap(s.Config)
	return s
}
func copyMap(v map[string]any) map[string]any {
	b, _ := json.Marshal(v)
	out := map[string]any{}
	_ = json.Unmarshal(b, &out)
	if out == nil {
		out = map[string]any{}
	}
	return out
}

// Configure persists validated settings; restart the instance to apply its snapshot.
func (m *Manager) Configure(id, bot string, config map[string]any) error {
	mf, _, err := m.manifest(id)
	if err != nil {
		return err
	}
	if _, err = json.Marshal(config); err != nil {
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
