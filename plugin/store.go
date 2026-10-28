package plugin

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type Plugin struct {
	ID           string           `json:"id"`
	Name         string           `json:"name"`
	Description  string           `json:"description"`
	Enabled      bool             `json:"enabled"`
	BotID        string           `json:"bot_id"`
	Graph        json.RawMessage  `json:"graph"`
	CustomNodes  json.RawMessage  `json:"custom_nodes,omitempty"`
	CreatedAt    int64            `json:"created_at"`
	UpdatedAt    int64            `json:"updated_at"`
}

type PluginStore struct {
	Plugins     map[string]*Plugin
	mu         sync.RWMutex
	dataDir    string
}

func NewPluginStore(dataDir string) (*PluginStore, error) {
	store := &PluginStore{
		Plugins: make(map[string]*Plugin),
		dataDir: dataDir,
	}
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create plugin data dir: %w", err)
	}
	if err := store.loadAll(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *PluginStore) loadAll() error {
	entries, err := os.ReadDir(s.dataDir)
	if err != nil {
		return nil
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		data, err := os.ReadFile(filepath.Join(s.dataDir, entry.Name()))
		if err != nil {
			continue
		}

		var p Plugin
		if err := json.Unmarshal(data, &p); err != nil {
			continue
		}
		s.Plugins[p.ID] = &p
	}
	return nil
}

func (s *PluginStore) List() []*Plugin {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*Plugin, 0, len(s.Plugins))
	for _, p := range s.Plugins {
		result = append(result, p)
	}
	return result
}

func (s *PluginStore) Get(id string) (*Plugin, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.Plugins[id]
	return p, ok
}

func (s *PluginStore) Save(p *Plugin) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Plugins[p.ID] = p

	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal plugin: %w", err)
	}

	filePath := filepath.Join(s.dataDir, p.ID+".json")
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write plugin file: %w", err)
	}
	return nil
}

func (s *PluginStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.Plugins, id)

	filePath := filepath.Join(s.dataDir, id+".json")
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete plugin file: %w", err)
	}
	return nil
}

func (s *PluginStore) GetEnabledPlugins(botID string) []*Plugin {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*Plugin, 0)
	for _, p := range s.Plugins {
		if p.Enabled && (p.BotID == "" || p.BotID == botID || p.BotID == "*") {
			result = append(result, p)
		}
	}
	return result
}

// GetPluginsByIDs 按插件ID列表获取插件（只返回存在的插件，不论启用状态）
func (s *PluginStore) GetPluginsByIDs(ids []string) []*Plugin {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*Plugin, 0, len(ids))
	for _, id := range ids {
		if p, ok := s.Plugins[id]; ok {
			result = append(result, p)
		}
	}
	return result
}

// GetEnabledPluginsByIDs 按插件ID列表获取已启用的插件
func (s *PluginStore) GetEnabledPluginsByIDs(ids []string) []*Plugin {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*Plugin, 0, len(ids))
	for _, id := range ids {
		if p, ok := s.Plugins[id]; ok && p.Enabled {
			result = append(result, p)
		}
	}
	return result
}