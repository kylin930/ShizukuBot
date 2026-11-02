// Package example demonstrates advanced lifecycle hooks and per-Bot cached state.
package example

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"shizukubot/sdk"
)

// Plugin caches the persisted counter for one Bot. The host serializes commands
// and lifecycle hooks; no mutable state is shared across constructed instances.
type Plugin struct {
	sdk.Base
	host   sdk.Host
	count  int64
	prefix string
	ready  bool
}

// Definition keeps the advanced, explicit Manifest and fresh-instance factory.
func Definition() sdk.Definition {
	return sdk.Definition{
		Manifest: sdk.Manifest{
			APIVersion: sdk.APIVersion, ID: "counter", Name: "独立计数器", Version: "1.0.0", Runtime: "go",
			Permissions:  []string{"commands.register", "message.send", "config.read", "state.read", "state.write"},
			ConfigSchema: sdk.Schema{Type: "object", Properties: map[string]sdk.Schema{"prefix": {Description: "计数回复的文字前缀", Type: "string", Default: []byte(`"计数"`)}}},
		},
		New: func() sdk.Extension { return &Plugin{} },
	}
}

// Load stores the instance host and registers a bound method, not a global object.
func (p *Plugin) Load(_ context.Context, h sdk.Host) error {
	p.host = h
	return h.Command("count", p.Count)
}

// Start restores state once; malformed persisted data is reported at startup.
func (p *Plugin) Start(ctx context.Context) error {
	raw, err := p.host.State().Get(ctx, "count")
	if err != nil {
		return err
	}
	p.count = 0
	if len(raw) > 0 {
		if err = json.Unmarshal(raw, &p.count); err != nil {
			return err
		}
	}
	p.prefix, _ = p.host.Config()["prefix"].(string)
	p.ready = true
	return nil
}

// Count persists before replying. A failed write does not advance the cached value.
func (p *Plugin) Count(ctx context.Context, c sdk.Call) error {
	if !p.ready {
		return errors.New("counter is not started")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	next := p.count + 1
	raw, err := json.Marshal(next)
	if err != nil {
		return err
	}
	if err = c.State.Set(ctx, "count", raw); err != nil {
		return err
	}
	p.count = next
	return c.Reply(ctx, fmt.Sprintf("%s: %d", p.prefix, next))
}

// Stop ends command use; every update has already been persisted.
func (p *Plugin) Stop(context.Context) error { p.ready = false; return nil }

// Unload releases references even after partial initialization failure.
func (p *Plugin) Unload(context.Context) error { p.host = nil; return nil }
