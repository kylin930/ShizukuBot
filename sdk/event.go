package sdk

import (
	"shizukubot/events"
	"unicode"
	"unicode/utf8"
)

// Event is the shared transport-neutral event, not a copy or a QQ SDK DTO.
type Event = events.Event

// EventType identifies a canonical event. Custom types remain supported.
type EventType = events.Type

// Reply is a host-adapted outgoing text or media request.
type Reply = events.Reply

// Action is a host-adapted management request.
type Action = events.Action

// ActionKind identifies a management capability.
type ActionKind = events.ActionKind

const (
	ActionRecall = events.ActionRecall
	ActionMute   = events.ActionMute
	ActionUnmute = events.ActionUnmute
	ActionKick   = events.ActionKick
)

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

// ValidEventType accepts bounded nonempty canonical/custom event names without
// whitespace or control characters. It does not restrict names to built-in types.
func ValidEventType(typ EventType) bool {
	if len(typ) == 0 || len(typ) > 128 || !utf8.ValidString(string(typ)) {
		return false
	}
	for _, r := range typ {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return false
		}
	}
	return true
}
