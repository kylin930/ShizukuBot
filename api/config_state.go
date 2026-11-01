package api

import (
	"shizukubot/config"
)

// configSnapshot gives queries an independent view; no caller holds a lock over external work.
func (r *Router) configSnapshot() *config.Config {
	r.configMu.RLock()
	defer r.configMu.RUnlock()
	return cloneConfig(r.config)
}
func cloneConfig(c *config.Config) *config.Config {
	out := *c
	out.Bots = append([]config.BotConfig(nil), c.Bots...)
	for i := range out.Bots {
		out.Bots[i].Plugins = append([]string(nil), c.Bots[i].Plugins...)
		out.Bots[i].AdminUsers = append([]string(nil), c.Bots[i].AdminUsers...)
	}
	return &out
}

func (r *Router) changeConfig(change func(*config.Config)) error {
	r.configMu.Lock()
	defer r.configMu.Unlock()
	next := cloneConfig(r.config)
	change(next)
	if err := config.SaveConfig(r.configPath, next); err != nil {
		return err
	}
	r.config = next
	return nil
}
