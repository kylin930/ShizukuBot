// Package qq implements QQ transport adaptation without global SDK callbacks.
package qq

import (
	"encoding/json"
	"fmt"
	"shizukubot/events"
)

// Normalize is shared by signed HTTP callbacks and WebSocket dispatch packets.
func Normalize(botID string, raw []byte) (events.Event, error) {
	var p struct {
		ID   string          `json:"id"`
		Type string          `json:"t"`
		Data json.RawMessage `json:"d"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return events.Event{}, err
	}
	var d struct {
		ID          string `json:"id"`
		Content     string `json:"content"`
		GroupID     string `json:"group_id"`
		GroupOpenID string `json:"group_openid"`
		ChannelID   string `json:"channel_id"`
		GuildID     string `json:"guild_id"`
		OpenID      string `json:"openid"`
		OpMember    string `json:"op_member_openid"`
		Author      *struct {
			ID           string `json:"id"`
			UserOpenID   string `json:"user_openid"`
			MemberOpenID string `json:"member_openid"`
		} `json:"author"`
	}
	if err := json.Unmarshal(p.Data, &d); err != nil {
		return events.Event{}, fmt.Errorf("invalid event data: %w", err)
	}
	e := events.Event{BotID: botID, EventID: p.ID, PlatformType: p.Type, Raw: append([]byte(nil), p.Data...), Message: d.Content, GroupID: d.GroupID, ChannelID: d.ChannelID, GuildID: d.GuildID, UserID: d.OpenID}
	if e.GroupID == "" {
		e.GroupID = d.GroupOpenID
	}
	if d.Author != nil {
		e.UserID = d.Author.ID
		if e.UserID == "" {
			e.UserID = d.Author.UserOpenID
		}
		if e.UserID == "" {
			e.UserID = d.Author.MemberOpenID
		}
	}
	switch p.Type {
	case "GROUP_AT_MESSAGE_CREATE":
		e.Type = "group_at_message"
		e.Source = events.Group
	case "GROUP_MESSAGE_CREATE":
		e.Type = "group_message"
		e.Source = events.Group
	case "C2C_MESSAGE_CREATE":
		e.Type = "c2c_message"
		e.Source = events.C2C
	case "AT_MESSAGE_CREATE":
		e.Type = "at_message"
		e.Source = events.Guild
	case "MESSAGE_CREATE":
		e.Type = "guild_message"
		e.Source = events.Guild
	case "DIRECT_MESSAGE_CREATE":
		e.Type = "direct_message"
		e.Source = events.Direct
	case "INTERACTION_CREATE":
		e.Type = "interaction"
	case "FRIEND_ADD", "FRIEND_DEL", "C2C_FRIEND_ADD", "C2C_FRIEND_DEL":
		e.Type = "c2c_friend"
		e.Source = events.C2C
	case "SUBSCRIBE_MESSAGE_STATUS":
		e.Type = "subscribe_msg_status"
	case "GROUP_ADD_ROBOT", "GROUP_DEL_ROBOT", "GROUP_MSG_REJECT", "GROUP_MSG_RECEIVE":
		e.Type = "group_manage"
		e.Source = events.Group
		e.UserID = d.OpMember
	case "C2C_MSG_REJECT", "C2C_MSG_RECEIVE":
		e.Type = "c2c_manage"
		e.Source = events.C2C
	default:
		e.Type = events.Type(p.Type)
	}
	switch e.Type {
	case "group_at_message", "group_message", "c2c_message", "at_message", "guild_message", "direct_message":
		e.MessageID = d.ID
	}
	return e, nil
}
