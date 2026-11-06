package sdk

import (
	"encoding/json"
	"regexp"
	"shizukubot/events"
	"strings"
)

var (
	botMention    = regexp.MustCompile(`<@!?[0-9A-Fa-f]+>`)
	qqUserMention = regexp.MustCompile(`<qqbot-at-user id="([^"]*)"\s*/>`)
)

// MentionUserIDs returns user IDs mentioned by the message, excluding the bot
// itself when the raw event marks the mention with is_you. Duplicate IDs are
// removed while preserving first-seen order.
func MentionUserIDs(e Event) []string {
	seen := map[string]bool{}
	ids := []string{}
	add := func(id string) {
		if id != "" && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	for _, m := range qqUserMention.FindAllStringSubmatch(e.Message, -1) {
		add(m[1])
	}
	for _, m := range botMention.FindAllString(e.Message, -1) {
		id := strings.TrimPrefix(strings.TrimSuffix(strings.TrimPrefix(m, "<@"), ">"), "!")
		add(id)
	}
	var d struct {
		Mentions []struct {
			ID    string `json:"id"`
			IsYou bool   `json:"is_you"`
		} `json:"mentions"`
	}
	_ = json.Unmarshal(e.Raw, &d)
	for _, u := range d.Mentions {
		if !u.IsYou {
			add(u.ID)
		}
	}
	return ids
}

func mentionIDs(e events.Event) []string {
	var d struct {
		Mentions []struct {
			ID    string `json:"id"`
			IsYou bool   `json:"is_you"`
		} `json:"mentions"`
	}
	_ = json.Unmarshal(e.Raw, &d)
	ids := []string{}
	for _, u := range d.Mentions {
		if u.IsYou {
			ids = append(ids, u.ID)
		}
	}
	return ids
}

// CommandText removes only known bot mentions, falling back to historical QQ mention syntax.
func CommandText(e events.Event) string {
	text := e.Message
	ids := mentionIDs(e)
	if len(ids) == 0 {
		text = botMention.ReplaceAllString(text, "")
	} else {
		for _, id := range ids {
			text = strings.ReplaceAll(strings.ReplaceAll(text, "<@"+id+">", ""), "<@!"+id+">", "")
		}
	}
	text = qqUserMention.ReplaceAllString(text, "")
	return strings.Trim(text, " \u00a0")
}
