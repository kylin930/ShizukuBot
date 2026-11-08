package qq

import (
	"shizukubot/events"
	"testing"
)

func TestNormalizeSourcesAndManagement(t *testing.T) {
	cases := []struct {
		typ, canonical string
		source         events.Source
	}{
		{"GROUP_AT_MESSAGE_CREATE", "group_at_message", events.Group}, {"GROUP_MESSAGE_CREATE", "group_message", events.Group}, {"C2C_MESSAGE_CREATE", "c2c_message", events.C2C}, {"AT_MESSAGE_CREATE", "at_message", events.Guild}, {"DIRECT_MESSAGE_CREATE", "direct_message", events.Direct}, {"INTERACTION_CREATE", "interaction", ""}, {"GROUP_DEL_ROBOT", "group_manage", events.Group}, {"C2C_MSG_REJECT", "c2c_manage", events.C2C}, {"FRIEND_ADD", "c2c_friend", events.C2C}, {"SUBSCRIBE_MESSAGE_STATUS", "subscribe_msg_status", ""},
	}
	for _, tt := range cases {
		t.Run(tt.typ, func(t *testing.T) {
			raw := []byte(`{"id":"event","t":"` + tt.typ + `","d":{"id":"message","group_openid":"g","author":{"member_openid":"u"}}}`)
			e, err := Normalize("bot", raw)
			if err != nil || string(e.Type) != tt.canonical || e.Source != tt.source || e.BotID != "bot" || e.EventID != "event" {
				t.Fatalf("%+v %v", e, err)
			}
		})
	}
	if _, err := Normalize("bot", []byte(`{"d":false}`)); err == nil {
		t.Fatal("invalid data accepted")
	}
}

func TestNormalizeSenderNickname(t *testing.T) {
	e, err := Normalize("bot", []byte(`{"id":"event","t":"GROUP_AT_MESSAGE_CREATE","d":{"id":"message","group_openid":"group","author":{"member_openid":"member","username":"小明"}}}`))
	if err != nil || e.Nickname != "小明" || e.UserID != "member" {
		t.Fatalf("%+v %v", e, err)
	}
}

func TestQQIdentifierNamespacesAndUnsupportedInteraction(t *testing.T) {
	for _, kind := range []string{"C2C_MESSAGE_CREATE", "GROUP_AT_MESSAGE_CREATE", "GROUP_MESSAGE_CREATE"} {
		e, err := Normalize("bot", []byte(`{"id":"event","t":"`+kind+`","d":{"id":"message","group_id":"legacy","group_openid":"group","author":{"id":"guild-user","user_openid":"c2c-user","member_openid":"group-member"}}}`))
		if err != nil || e.MessageID != "message" || e.EventID != "event" {
			t.Fatal(e, err)
		}
		if kind == "C2C_MESSAGE_CREATE" {
			if e.UserID != "c2c-user" {
				t.Fatal(e)
			}
		} else if e.GroupID != "group" || e.UserID != "group-member" {
			t.Fatal(e)
		}
	}
	e, err := Normalize("bot", []byte(`{"id":"event","t":"INTERACTION_CREATE","d":{"id":"interaction","chat_type":1,"group_openid":"group","user_openid":"user"}}`))
	if err != nil || e.MessageID != "" || e.Source != "" {
		t.Fatal("interaction gained guessed message capability", e, err)
	}
}
