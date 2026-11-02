package sdk

import (
	"encoding/json"
	"regexp"
	"shizukubot/events"
	"strings"
)

var botMention = regexp.MustCompile(`<@!?[0-9A-Fa-f]+>`)

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
	return strings.Trim(text, " \u00a0")
}
