package hello

import (
	"context"
	"shizukubot/sdk"
)

// Definition registers /ping without a custom struct or lifecycle implementation.
func Definition() sdk.Definition {
	return sdk.CommandPlugin(sdk.Meta{ID: "hello", Name: "Ping / Pong", Version: "1.0.0"}, "ping", Ping)
}

// Ping can be tested with a capture-only Call without starting the framework.
func Ping(ctx context.Context, c sdk.Call) error { return c.Reply(ctx, "pong") }
