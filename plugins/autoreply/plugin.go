// Package autoreply demonstrates configurable message event subscriptions.
package autoreply

import (
	"context"
	"shizukubot/events"
	sdk "shizukubot/plugins"
	"strings"
)

type Plugin struct{ sdk.Base }

func Definition() sdk.Definition {
	return sdk.Definition{Manifest: sdk.Manifest{APIVersion: sdk.APIVersion, ID: "autoreply", Name: "关键词自动回复", Version: "1.0.0", Runtime: "go", Permissions: []string{"events.subscribe", "message.send", "config.read"}, ConfigSchema: sdk.Schema{Type: "object", Properties: map[string]sdk.Schema{"keyword": {Type: "string", Default: []byte(`"你好"`)}, "reply": {Type: "string", Default: []byte(`"你好！"`)}}}}, New: func() sdk.Extension { return &Plugin{} }}
}
func (*Plugin) Load(_ context.Context, h sdk.Host) error {
	for _, typ := range []events.Type{"group_message", "group_at_message", "c2c_message"} {
		if err := h.On(typ, Respond); err != nil {
			return err
		}
	}
	return nil
}
func Respond(ctx context.Context, c sdk.Call) error {
	keyword, _ := c.Config["keyword"].(string)
	reply, _ := c.Config["reply"].(string)
	if keyword != "" && strings.Contains(c.Event.Message, keyword) {
		return c.Reply(ctx, reply)
	}
	return nil
}
