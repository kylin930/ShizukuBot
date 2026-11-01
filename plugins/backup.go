package plugins

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
)

type botBackup struct {
	Config map[string]any             `json:"config"`
	State  map[string]json.RawMessage `json:"state"`
}
type packageBackup struct {
	Version int                  `json:"version"`
	Bots    map[string]botBackup `json:"bots"`
}

// ExportBackup includes code and private plugin data. Activation/trust are deliberately excluded.
func (m *Manager) ExportBackup(id string) ([]byte, error) {
	code, err := m.Store.Export(id)
	if err != nil {
		return nil, err
	}
	metadata := packageBackup{Version: 1, Bots: map[string]botBackup{}}
	m.settingsMu.Lock()
	scopes := map[string]Settings{}
	for bot, s := range m.settings[id] {
		s.Config = copyMap(s.Config)
		scopes[bot] = s
	}
	m.settingsMu.Unlock()
	for bot, s := range scopes {
		state := &diskState{file: filepath.Join(m.root, "state", id, scope(bot)+".json")}
		values, err := state.read()
		if err != nil {
			return nil, err
		}
		metadata.Bots[bot] = botBackup{Config: s.Config, State: values}
	}
	raw, err := json.Marshal(metadata)
	if err != nil {
		return nil, err
	}
	if len(raw) > 1<<20 {
		return nil, errors.New("backup metadata too large; use an offline filesystem backup")
	}
	z, err := zip.NewReader(bytes.NewReader(code), int64(len(code)))
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	writer := zip.NewWriter(&out)
	for _, f := range z.File {
		if f.Name == "backup.json" {
			continue
		}
		reader, err := f.Open()
		if err != nil {
			return nil, err
		}
		b, err := io.ReadAll(reader)
		reader.Close()
		if err != nil {
			return nil, err
		}
		w, err := writer.Create(f.Name)
		if err != nil {
			return nil, err
		}
		if _, err = w.Write(b); err != nil {
			return nil, err
		}
	}
	w, err := writer.Create("backup.json")
	if err != nil {
		return nil, err
	}
	if _, err = w.Write(raw); err != nil {
		return nil, err
	}
	if err = writer.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// InstallPackage optionally restores explicit backup data into unused scopes. It never trusts code.
func (m *Manager) InstallPackage(data []byte, restoreData bool) (Package, error) {
	if len(data) > 4<<20 {
		return Package{}, errors.New("package too large")
	}
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return Package{}, err
	}
	if len(z.File) > 128 {
		return Package{}, errors.New("too many files")
	}
	var expanded uint64
	for _, f := range z.File {
		expanded += f.UncompressedSize64
		if f.UncompressedSize64 > 1<<20 || expanded > 4<<20 {
			return Package{}, errors.New("expanded package exceeds quota")
		}
	}
	var mf Manifest
	var backup packageBackup
	hasBackup := false
	for _, f := range z.File {
		if f.Name != "manifest.json" && f.Name != "backup.json" {
			continue
		}
		r, err := f.Open()
		if err != nil {
			return Package{}, err
		}
		b, err := io.ReadAll(io.LimitReader(r, (1<<20)+1))
		r.Close()
		if err != nil || len(b) > 1<<20 {
			return Package{}, errors.New("invalid package metadata")
		}
		if f.Name == "manifest.json" {
			if err = json.Unmarshal(b, &mf); err != nil {
				return Package{}, err
			}
		} else {
			hasBackup = true
			if err = json.Unmarshal(b, &backup); err != nil {
				return Package{}, err
			}
		}
	}
	if err = mf.Validate(); err != nil {
		return Package{}, err
	}
	m.mu.RLock()
	_, reserved := m.native[mf.ID]
	m.mu.RUnlock()
	if reserved {
		return Package{}, errors.New("ID reserved for compiled Go extension")
	}
	if restoreData {
		if !hasBackup || backup.Version != 1 || len(backup.Bots) > 32 {
			return Package{}, errors.New("invalid backup metadata")
		}
		for bot, b := range backup.Bots {
			if bot == "" || len(bot) > 256 {
				return Package{}, errors.New("invalid bot scope")
			}
			validated, err := mf.ConfigSchema.Validate(b.Config)
			if err != nil {
				return Package{}, err
			}
			b.Config = validated.(map[string]any)
			for key, value := range b.State {
				if key == "" || len(key) > 128 || !json.Valid(value) || len(value) > 65536 {
					return Package{}, errors.New("invalid backup state")
				}
			}
			backup.Bots[bot] = b
		}
	}
	// Keep metadata out of executable projects so later code-only exports cannot leak it.
	var code bytes.Buffer
	writer := zip.NewWriter(&code)
	for _, f := range z.File {
		if f.Name == "backup.json" {
			continue
		}
		header := f.FileHeader
		w, err := writer.CreateHeader(&header)
		if err != nil {
			return Package{}, err
		}
		r, err := f.Open()
		if err != nil {
			return Package{}, err
		}
		_, err = io.Copy(w, io.LimitReader(r, (1<<20)+1))
		r.Close()
		if err != nil {
			return Package{}, err
		}
	}
	if err = writer.Close(); err != nil {
		return Package{}, err
	}
	if !restoreData {
		return m.Store.Install(code.Bytes())
	}
	m.settingsMu.Lock()
	defer m.settingsMu.Unlock()
	if len(m.settings[mf.ID]) > 0 {
		return Package{}, errors.New("existing plugin data preserved; restore into an empty data root")
	}
	stateDir := filepath.Join(m.root, "state", mf.ID)
	if _, err = os.Lstat(stateDir); !os.IsNotExist(err) {
		return Package{}, errors.New("existing state directory preserved")
	}
	p, err := m.Store.Install(code.Bytes())
	if err != nil {
		return Package{}, err
	}
	rollback := func() { _ = m.Store.Remove(mf.ID); _ = os.RemoveAll(stateDir); delete(m.settings, mf.ID) }
	m.settings[mf.ID] = map[string]Settings{}
	for bot, b := range backup.Bots {
		if err = AtomicJSON(filepath.Join(stateDir, scope(bot)+".json"), b.State); err != nil {
			rollback()
			return Package{}, err
		}
		m.settings[mf.ID][bot] = Settings{Config: b.Config}
	}
	if err = AtomicJSON(filepath.Join(m.root, "settings.json"), m.settings); err != nil {
		rollback()
		return Package{}, err
	}
	return p, nil
}
