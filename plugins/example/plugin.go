// Package example demonstrates persistent per-Bot state and validated configuration.
package example

import (
	"context"
	"encoding/json"
	"fmt"
	sdk "shizukubot/plugins"
)

type Plugin struct{ sdk.Base }

func Definition() sdk.Definition {
	return sdk.Definition{Manifest: sdk.Manifest{APIVersion: sdk.APIVersion, ID: "counter", Name: "独立计数器", Version: "1.0.0", Runtime: "go", Permissions: []string{"commands.register", "message.send", "config.read", "state.read", "state.write"}, ConfigSchema: sdk.Schema{Type: "object", Properties: map[string]sdk.Schema{"prefix": {Type: "string", Default: []byte(`"计数"`)}}}}, New: func() sdk.Extension { return &Plugin{} }}
}
func (*Plugin) Load(_ context.Context, h sdk.Host) error { return h.Command("count", Count) }

// Count relies on the Manager's serial execution per plugin/Bot instance.
func Count(ctx context.Context, c sdk.Call) error {
	raw, err := c.State.Get(ctx, "count")
	if err != nil {
		return err
	}
	var n int64
	if len(raw) > 0 {
		if err = json.Unmarshal(raw, &n); err != nil {
			return err
		}
	}
	n++
	raw, err = json.Marshal(n)
	if err != nil {
		return err
	}
	if err = c.State.Set(ctx, "count", raw); err != nil {
		return err
	}
	prefix, _ := c.Config["prefix"].(string)
	return c.Reply(ctx, fmt.Sprintf("%s: %d", prefix, n))
}
