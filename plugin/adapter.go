package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/tencent-connect/botgo/dto"
	"shizukubot/events"
)

// LegacyAdapter is the only graph-specific bridge from the v2 event bus.
// It executes trusted legacy code in-process; it is not a security sandbox.
type LegacyAdapter struct {
	Engine   *ExecutionEngine
	LogDebug func(string, string)
}

func (a *LegacyAdapter) Handle(ctx context.Context, e events.Event) error {
	return a.Execute(ctx, e, "")
}

// ExecutionResult records each legacy invocation without exposing interpreter details.
type ExecutionResult struct {
	PluginID string `json:"plugin_id"`
	Matched  bool   `json:"matched"`
	Status   string `json:"status"`
}

// Execute selects a legacy plugin explicitly for debugging, or matches enabled plugins.
func (a *LegacyAdapter) Execute(ctx context.Context, e events.Event, pluginID string) error {
	_, err := a.Run(ctx, e, pluginID)
	return err
}

// Run returns invocation outcomes for debugging and preserves all handler errors.
func (a *LegacyAdapter) Run(ctx context.Context, e events.Event, pluginID string) ([]ExecutionResult, error) {
	var data any
	switch e.Type {
	case "group_at_message":
		data = &dto.WSGroupATMessageData{}
	case "group_message":
		data = &dto.WSGroupMessageData{}
	case "c2c_message":
		data = &dto.WSC2CMessageData{}
	case "at_message", "guild_message":
		data = &dto.WSATMessageData{}
	case "direct_message":
		data = &dto.WSDirectMessageData{}
	case "c2c_friend":
		data = &dto.WSC2CFriendData{}
	case "subscribe_msg_status":
		data = &dto.WSSubscribeMsgStatus{}
	case "interaction":
		data = &dto.WSInteractionData{}
	case "group_manage":
		data = &dto.WSGroupManageData{}
	case "c2c_manage":
		data = &dto.WSC2CManageData{}
	}
	if data != nil && len(e.Raw) > 0 {
		if err := json.Unmarshal(e.Raw, data); err != nil {
			return nil, fmt.Errorf("decode legacy event: %w", err)
		}
	}
	c := ConvertWSDataToContext(e.BotID, string(e.Type), data, nil, nil, a.LogDebug)
	c.Context = ctx
	c.Message = e.Message
	c.MessageID = e.MessageID
	c.UserID = e.UserID
	c.GroupID = e.GroupID
	c.ChannelID = e.ChannelID
	c.GuildID = e.GuildID
	c.PluginIDs = e.PluginIDs
	c.Variables["event_type"] = e.PlatformType
	send := func(r events.Reply) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if e.Reply == nil {
			return errors.New("reply capability unavailable")
		}
		return e.Reply(ctx, r)
	}
	c.ReplyToUser = func(s string) error { return send(events.Reply{Kind: "text", Text: s}) }
	c.ReplyToGroup = c.ReplyToUser
	c.ReplyImageToUser = func(s string) error { return send(events.Reply{Kind: "image", URL: s}) }
	c.ReplyImageToGroup = c.ReplyImageToUser
	c.ReplyRichMediaToUser = func(s, u string) error { return send(events.Reply{Kind: "rich_media", Text: s, URL: u}) }
	c.ReplyRichMediaToGroup = c.ReplyRichMediaToUser
	c.ReplyMarkdownToUser = func(s string) error { return send(events.Reply{Kind: "markdown", Text: s}) }
	c.ReplyMarkdownToGroup = c.ReplyMarkdownToUser
	c.ReplyKeyboardToUser = func(s, k string) error { return send(events.Reply{Kind: "keyboard", Text: s, Keyboard: k}) }
	c.ReplyKeyboardToGroup = c.ReplyKeyboardToUser
	c.ReplyVideoToUser = func(s string) error { return send(events.Reply{Kind: "video", URL: s}) }
	c.ReplyVideoToGroup = c.ReplyVideoToUser
	plugins := a.Engine.getPluginsForExecution(c)
	if pluginID != "" {
		p, ok := a.Engine.store.Get(pluginID)
		if !ok {
			return nil, errors.New("plugin not found")
		}
		plugins = []*Plugin{p}
	}
	var errs []error
	var results []ExecutionResult
	for _, p := range plugins {
		if err := ctx.Err(); err != nil {
			return results, errors.Join(append(errs, err)...)
		}
		matched, err := a.executeSafe(c, p, pluginID != "")
		status := "completed"
		if !matched && pluginID == "" {
			status = "skipped"
		}
		if err != nil {
			status = "failed"
			errs = append(errs, fmt.Errorf("plugin %s: %w", p.ID, err))
		}
		results = append(results, ExecutionResult{PluginID: p.ID, Matched: matched, Status: status})
	}
	return results, errors.Join(errs...)
}
func (a *LegacyAdapter) executeSafe(c *EventContext, p *Plugin, force bool) (matched bool, err error) {
	defer func() {
		if recover() != nil {
			err = errors.New("legacy plugin panicked")
		}
	}()
	matched = a.Engine.hasEventNodeHit(p, c)
	if !force && !matched {
		return matched, nil
	}
	return matched, a.Engine.ExecutePlugin(c, p)
}
