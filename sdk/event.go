package sdk

import "shizukubot/events"

// Event is the shared transport-neutral event, not a copy or a QQ SDK DTO.
type Event = events.Event

// EventType identifies a canonical event. Custom types remain supported.
type EventType = events.Type

// Reply is a host-adapted outgoing text or media request.
type Reply = events.Reply

// Canonical event names match the existing QQ normalizer. GroupAtMessage is
// distinct from GroupMessage; subscribe to both to receive both push types.
const (
	GroupMessage           EventType = "group_message"
	GroupAtMessage         EventType = "group_at_message"
	C2CMessage             EventType = "c2c_message"
	GuildMessage           EventType = "guild_message"
	AtMessage              EventType = "at_message"
	DirectMessage          EventType = "direct_message"
	Interaction            EventType = "interaction"
	C2CFriend              EventType = "c2c_friend"
	SubscribeMessageStatus EventType = "subscribe_msg_status"
	GroupManage            EventType = "group_manage"
	C2CManage              EventType = "c2c_manage"
	AnyEvent               EventType = "*"
)
