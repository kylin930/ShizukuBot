package core

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/kylin930/botgo-plus/dto"
	"shizukubot/events"
)

// qqGroupAPIBase is a variable so management calls can be pointed at a test server.
var qqGroupAPIBase = "https://api.sgroup.qq.com"

const maxGroupMuteSeconds = 30 * 24 * 60 * 60

// actionContext binds management capabilities to one admitted event. Routing
// data is captured; message text and raw payload are deliberately discarded.
func (b *Bot) actionContext(life context.Context, e events.Event) func(context.Context, events.Action) error {
	e.Message, e.Raw, e.Reply, e.Act, e.PluginIDs = "", nil, nil, nil, nil
	return func(ctx context.Context, a events.Action) error {
		call, cancel := context.WithCancel(ctx)
		stop := context.AfterFunc(life, cancel)
		defer stop()
		defer cancel()
		if err := life.Err(); err != nil {
			return err
		}
		if err := call.Err(); err != nil {
			return err
		}
		switch e.Source {
		case events.Group:
			return b.groupAction(call, e, a)
		case events.C2C:
			return b.c2cAction(call, e, a)
		case events.Guild:
			return b.guildAction(call, e, a)
		case events.Direct:
			return b.directAction(call, e, a)
		default:
			return errors.New("management action source unsupported")
		}
	}
}

func (b *Bot) groupAction(ctx context.Context, e events.Event, a events.Action) error {
	switch a.Kind {
	case events.ActionRecall:
		id := a.MessageID
		if id == "" {
			id = e.MessageID
		}
		if e.GroupID == "" || id == "" {
			return errors.New("recall requires a group and message ID")
		}
		return safeQQError(b.OpenAPI.RetractGroupMessage(ctx, e.GroupID, id))
	case events.ActionMute, events.ActionUnmute:
		if e.GroupID == "" || a.UserID == "" {
			return errors.New("mute requires a group and member_openid")
		}
		member := map[string]any{"op": "add", "member_openid": a.UserID}
		if a.Kind == events.ActionUnmute {
			member["op"] = "del"
		} else {
			if a.Seconds <= 0 || a.Seconds > maxGroupMuteSeconds {
				return errors.New("group mute duration must be between 1 second and 30 days")
			}
			member["mute_expire_at"] = time.Now().In(time.FixedZone("CST", 8*3600)).
				Add(time.Duration(a.Seconds) * time.Second).Format(time.RFC3339)
		}
		body := map[string]any{"members": []any{member}}
		return b.groupTransport(ctx, http.MethodPost, e.GroupID, "/restrict_chat_setting", body)
	case events.ActionKick:
		if e.GroupID == "" || a.UserID == "" {
			return errors.New("kick requires a group and member_openid")
		}
		body := map[string]any{"member_openids": []string{a.UserID}, "add_to_member_blacklist": a.Blacklist}
		return b.groupTransport(ctx, http.MethodPost, e.GroupID, "/batch_remove_members", body)
	default:
		return errors.New("management action unsupported for group")
	}
}

func (b *Bot) c2cAction(ctx context.Context, e events.Event, a events.Action) error {
	if a.Kind != events.ActionRecall {
		return errors.New("management action unsupported for C2C")
	}
	id := a.MessageID
	if id == "" {
		id = e.MessageID
	}
	if e.UserID == "" || id == "" {
		return errors.New("recall requires a user and message ID")
	}
	return safeQQError(b.OpenAPI.RetractC2CMessage(ctx, e.UserID, id))
}

func (b *Bot) guildAction(ctx context.Context, e events.Event, a events.Action) error {
	switch a.Kind {
	case events.ActionRecall:
		id := a.MessageID
		if id == "" {
			id = e.MessageID
		}
		if e.ChannelID == "" || id == "" {
			return errors.New("recall requires a channel and message ID")
		}
		return safeQQError(b.OpenAPI.RetractMessage(ctx, e.ChannelID, id))
	case events.ActionMute, events.ActionUnmute:
		if e.GuildID == "" || a.UserID == "" {
			return errors.New("mute requires a guild and user ID")
		}
		seconds := a.Seconds
		if a.Kind == events.ActionUnmute {
			seconds = 0
		}
		return safeQQError(b.OpenAPI.MemberMute(ctx, e.GuildID, a.UserID,
			&dto.UpdateGuildMute{MuteSeconds: strconv.FormatInt(seconds, 10)}))
	case events.ActionKick:
		if e.GuildID == "" || a.UserID == "" {
			return errors.New("kick requires a guild and user ID")
		}
		return safeQQError(b.OpenAPI.DeleteGuildMember(ctx, e.GuildID, a.UserID))
	default:
		return errors.New("management action unsupported for guild")
	}
}

func (b *Bot) directAction(ctx context.Context, e events.Event, a events.Action) error {
	if a.Kind != events.ActionRecall {
		return errors.New("management action unsupported for direct messages")
	}
	id := a.MessageID
	if id == "" {
		id = e.MessageID
	}
	if e.GuildID == "" || id == "" {
		return errors.New("recall requires a guild and message ID")
	}
	return safeQQError(b.OpenAPI.RetractDMMessage(ctx, e.GuildID, id))
}

// groupTransport uses the SDK escape hatch for QQ group management endpoints
// that botgo-plus does not yet wrap. Authentication is handled by OpenAPI.
func (b *Bot) groupTransport(ctx context.Context, method, groupID, suffix string, body any) error {
	if groupID == "" {
		return errors.New("group ID missing")
	}
	endpoint := qqGroupAPIBase + "/v2/groups/" + url.PathEscape(groupID) + suffix
	if _, err := b.OpenAPI.Transport(ctx, method, endpoint, body); err != nil {
		return safeQQError(err)
	}
	return nil
}
