package sdk

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
)

// MessageEvents is the usual subscription set for chat plugins. EventPlugin
// copies the slice, so callers may share it safely.
var MessageEvents = []EventType{GroupMessage, GroupAtMessage, C2CMessage}

// Text returns the message with bot mentions removed. Chat plugins use it as
// their trigger source instead of Event.Message.
func Text(e Event) string { return CommandText(e) }

// Mentioned reports whether the bot itself was @-mentioned in the event.
func Mentioned(e Event) bool {
	if len(mentionIDs(e)) > 0 {
		return true
	}
	if len(e.Raw) > 0 {
		var d struct {
			Mentions []struct {
				IsYou bool `json:"is_you"`
			} `json:"mentions"`
		}
		if json.Unmarshal(e.Raw, &d) == nil {
			for _, m := range d.Mentions {
				if m.IsYou {
					return true
				}
			}
		}
	}
	return botMention.MatchString(e.Message)
}

// MatchPrefix reports whether text starts with prefix and returns the remainder
// with surrounding spaces trimmed.
func MatchPrefix(text, prefix string) (string, bool) {
	if !strings.HasPrefix(text, prefix) {
		return "", false
	}
	return strings.TrimSpace(text[len(prefix):]), true
}

// MatchSuffix reports whether text ends with suffix and returns the leading part
// with surrounding spaces trimmed.
func MatchSuffix(text, suffix string) (string, bool) {
	if !strings.HasSuffix(text, suffix) {
		return "", false
	}
	return strings.TrimSpace(text[:len(text)-len(suffix)]), true
}

// MatchFull reports whether the trimmed text equals target exactly.
func MatchFull(text, target string) bool { return strings.TrimSpace(text) == target }

// MatchRegexp returns the submatches when re matches text, or nil otherwise.
// Compile the expression once at package scope; this helper does not cache.
func MatchRegexp(text string, re *regexp.Regexp) []string {
	if re == nil {
		return nil
	}
	m := re.FindStringSubmatch(text)
	if m == nil {
		return nil
	}
	return m
}

// LoadJSON decodes key from state into v. A missing key is not an error and
// leaves v untouched, which lets callers keep their zero values as defaults.
func LoadJSON(ctx context.Context, st State, key string, v any) error {
	if st == nil {
		return nil
	}
	raw, err := st.Get(ctx, key)
	if err != nil {
		return err
	}
	if len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, v)
}

// SaveJSON marshals v and persists it under key.
func SaveJSON(ctx context.Context, st State, key string, v any) error {
	if st == nil {
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return st.Set(ctx, key, json.RawMessage(b))
}
