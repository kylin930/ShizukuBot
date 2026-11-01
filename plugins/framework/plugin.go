package framework

import (
	"context"
	"fmt"
	"runtime"
	"shizukubot/events"
	sdk "shizukubot/plugins"
)

// Definition retains the existing framework information command.
func Definition() sdk.Definition {
	return sdk.Definition{Manifest: sdk.Manifest{APIVersion: 1, ID: "builtin-info", Name: "Framework information (Go)", Version: "1.0.0", Runtime: "go", Permissions: []string{"commands.register", "message.send"}, ConfigSchema: sdk.Schema{Type: "object"}}, New: func() sdk.Extension { return &information{} }}
}

type information struct{}

func (*information) Load(_ context.Context, h sdk.Host) error {
	return h.Command("framework", func(ctx context.Context, c sdk.Call) error {
		return c.Event.Reply(ctx, events.Reply{Kind: "text", Text: fmt.Sprintf("ShizukuBot · %s/%s", runtime.GOOS, runtime.GOARCH)})
	})
}
func (*information) Start(context.Context) error                 { return nil }
func (*information) HandleEvent(context.Context, sdk.Call) error { return nil }
func (*information) Stop(context.Context) error                  { return nil }
func (*information) Unload(context.Context) error                { return nil }
