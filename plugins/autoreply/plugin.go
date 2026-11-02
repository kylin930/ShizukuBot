// Package autoreply demonstrates configurable message event subscriptions.
package autoreply

import (
	"context"
	"shizukubot/sdk"
	"strings"
)

// Definition listens to group and C2C messages with isolated Bot configuration.
func Definition() sdk.Definition {
	return sdk.EventPlugin(
		sdk.Meta{ID: "autoreply", Name: "关键词自动回复", Version: "1.0.0"},
		[]sdk.EventType{sdk.GroupMessage, sdk.GroupAtMessage, sdk.C2CMessage}, Respond,
		sdk.WithConfig(sdk.Schema{Type: "object", Properties: map[string]sdk.Schema{
			"keyword": {Description: "消息包含此关键词时回复，留空表示不回复", Type: "string", Default: []byte(`"你好"`)},
			"reply":   {Description: "关键词命中后发送的文本", Type: "string", Default: []byte(`"你好！"`)},
		}}),
	)
}

// Respond replies only when the configured, nonempty keyword matches.
func Respond(ctx context.Context, c sdk.Call) error {
	keyword, _ := c.Config["keyword"].(string)
	reply, _ := c.Config["reply"].(string)
	if keyword != "" && strings.Contains(c.Event.Message, keyword) {
		return c.Reply(ctx, reply)
	}
	return nil
}
