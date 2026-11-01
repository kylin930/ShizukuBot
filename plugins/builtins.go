package plugins

import (
	"context"
	"fmt"
	"runtime"
	"shizukubot/events"
)

// RegisterBuiltins is the static, cross-platform Go extension registry. Add trusted factories here.
// Registration is per Manager; no bot or handler state is shared globally.
func RegisterBuiltins(m *Manager) error {
	return m.RegisterGo(Manifest{APIVersion: 1, ID: "builtin-info", Name: "Framework information (Go)", Version: "1.0.0", Runtime: "go", Permissions: []string{"commands.register", "message.send"}, ConfigSchema: Schema{Type: "object"}}, func() Extension { return &information{} })
}

type information struct{}

func (*information) Load(_ context.Context, h Host) error {
	return h.Command("framework", func(ctx context.Context, c Call) error {
		return c.Event.Reply(ctx, events.Reply{Kind: "text", Text: fmt.Sprintf("ShizukuBot · %s/%s", runtime.GOOS, runtime.GOARCH)})
	})
}
func (*information) Start(context.Context) error             { return nil }
func (*information) HandleEvent(context.Context, Call) error { return nil }
func (*information) Stop(context.Context) error              { return nil }
func (*information) Unload(context.Context) error            { return nil }
