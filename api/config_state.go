package api

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"shizukubot/config"
)

var errConfigConflict = errors.New("配置已被其他管理员修改，请重新加载后重试")

func newConfigRevision() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("configuration revision randomness unavailable")
	}
	return hex.EncodeToString(b[:])
}
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
	return r.changeConfigChecked("", func(c *config.Config) error { change(c); return nil })
}
func (r *Router) changeConfigChecked(expected string, change func(*config.Config) error) error {
	r.configMu.Lock()
	defer r.configMu.Unlock()
	if expected != "" && expected != r.configRevision {
		return errConfigConflict
	}
	next := cloneConfig(r.config)
	if err := change(next); err != nil {
		return err
	}
	if err := config.SaveConfig(r.configPath, next); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	r.config = next
	r.configRevision = newConfigRevision()
	return nil
}
