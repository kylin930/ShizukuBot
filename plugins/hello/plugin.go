// Package hello implements the smallest command plugin.
package hello

import (
	"context"
	sdk "shizukubot/plugins"
)

type Plugin struct{ sdk.Base }

func Definition() sdk.Definition {
	return sdk.Definition{Manifest: sdk.Manifest{APIVersion: sdk.APIVersion, ID: "hello", Name: "Ping / Pong", Version: "1.0.0", Runtime: "go", Permissions: []string{"commands.register", "message.send"}, ConfigSchema: sdk.Schema{Type: "object"}}, New: func() sdk.Extension { return &Plugin{} }}
}
func (*Plugin) Load(_ context.Context, h sdk.Host) error { return h.Command("ping", Ping) }

// Ping is independently unit-testable without a QQ connection or Manager.
func Ping(ctx context.Context, c sdk.Call) error { return c.Reply(ctx, "pong") }
