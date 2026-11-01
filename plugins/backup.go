package plugins

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

type botBackup struct {
	Config map[string]any             `json:"config"`
	State  map[string]json.RawMessage `json:"state"`
}
type dataBackup struct {
	Version  int                  `json:"version"`
	PluginID string               `json:"plugin_id"`
	Bots     map[string]botBackup `json:"bots"`
}

// ExportBackup exports private config/state only. No executable code or activation is included.
func (m *Manager) ExportBackup(id string) ([]byte, error) {
	if _, _, err := m.manifest(id); err != nil {
		return nil, err
	}
	m.settingsMu.Lock()
	defer m.settingsMu.Unlock()
	backup := dataBackup{Version: 1, PluginID: id, Bots: map[string]botBackup{}}
	for bot, s := range m.settings[id] {
		state := &diskState{file: filepath.Join(m.root, "state", id, scope(bot)+".json")}
		values, err := state.read()
		if err != nil {
			return nil, err
		}
		backup.Bots[bot] = botBackup{Config: copyMap(s.Config), State: values}
	}
	raw, err := json.MarshalIndent(backup, "", "  ")
	if err != nil {
		return nil, err
	}
	if len(raw) > 4<<20 {
		return nil, errors.New("backup exceeds quota; use an offline backup")
	}
	return raw, nil
}

// RestoreBackup restores data for an already compiled plugin into an unused data scope.
// Existing data is never overwritten, and restored instances remain disabled.
func (m *Manager) RestoreBackup(raw []byte) error {
	if len(raw) > 4<<20 {
		return errors.New("backup exceeds quota")
	}
	var backup dataBackup
	if err := json.Unmarshal(raw, &backup); err != nil {
		return errors.New("invalid data backup")
	}
	mf, _, err := m.manifest(backup.PluginID)
	if err != nil {
		return err
	}
	if backup.Version != 1 || len(backup.Bots) > 32 {
		return errors.New("invalid backup version or scope count")
	}
	for bot, b := range backup.Bots {
		if bot == "" || len(bot) > 256 {
			return errors.New("invalid bot scope")
		}
		config, err := mf.ConfigSchema.Validate(b.Config)
		if err != nil {
			return err
		}
		b.Config = config.(map[string]any)
		for key, value := range b.State {
			if key == "" || len(key) > 128 || len(value) > 65536 || !json.Valid(value) {
				return errors.New("invalid backup state")
			}
		}
		state, _ := json.Marshal(b.State)
		if len(state) > 1<<20 {
			return errors.New("state quota exceeded")
		}
		backup.Bots[bot] = b
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, s := range m.slots {
		if s.status.PluginID == backup.PluginID {
			return errors.New("plugin has active or previous instances; use an empty data root")
		}
	}
	m.settingsMu.Lock()
	defer m.settingsMu.Unlock()
	if len(m.settings[backup.PluginID]) > 0 {
		return errors.New("existing configuration preserved")
	}
	stateDir := filepath.Join(m.root, "state", backup.PluginID)
	if _, err = os.Lstat(stateDir); !os.IsNotExist(err) {
		return errors.New("existing state preserved")
	}
	rollback := func(cause error) error {
		delete(m.settings, backup.PluginID)
		return errors.Join(cause, os.RemoveAll(stateDir))
	}
	m.settings[backup.PluginID] = map[string]Settings{}
	for bot, b := range backup.Bots {
		if err = AtomicJSON(filepath.Join(stateDir, scope(bot)+".json"), b.State); err != nil {
			return rollback(err)
		}
		m.settings[backup.PluginID][bot] = Settings{Config: b.Config}
	}
	if err = AtomicJSON(filepath.Join(m.root, "settings.json"), m.settings); err != nil {
		return rollback(err)
	}
	return nil
}
