// Package rules provides bounded text-response rules over transport-neutral events.
// It is data-driven; it does not load code or depend on the plugin SDK.
package rules

import (
	"errors"
	"fmt"
	"regexp"
	"shizukubot/events"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Resource limits are shared by validation, execution and the management API.
const (
	SchemaVersion    = 1
	MaxRules         = 100
	MaxKeywordBytes  = 1024
	MaxTemplateBytes = 4096
	MaxReplyBytes    = 8192
	MaxMessageBytes  = 65536
	MaxGroupIDs      = 32
	MaxIDBytes       = 128
	MaxReplyAttempts = 3
	MaxRequestBytes  = 1 << 20
)

var (
	ErrConflict    = errors.New("rule revision conflict")
	ErrClosed      = errors.New("rules engine closed")
	ErrReplyFailed = errors.New("rule text reply failed")
	idPattern      = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)
)

// Trigger matches the original, case-sensitive message, including whitespace.
type Trigger struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

// Conditions use AND. GroupIDs constrain group messages only, never C2C users.
type Conditions struct {
	Sources  []events.Source `json:"sources"`
	GroupIDs []string        `json:"group_ids"`
}

// Action supports only fixed text and four literal placeholder replacements.
type Action struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// Rule is scoped by its containing Set's BotID. IDs are unique within that Bot.
type Rule struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Enabled     bool       `json:"enabled"`
	Priority    int        `json:"priority"`
	Trigger     Trigger    `json:"trigger"`
	Conditions  Conditions `json:"conditions"`
	Action      Action     `json:"action"`
	StopOnMatch bool       `json:"stop_on_match"`
}

// Set is a versioned, optimistic-concurrency snapshot. Callers receive deep copies.
type Set struct {
	SchemaVersion int    `json:"schema_version"`
	BotID         string `json:"bot_id"`
	Revision      uint64 `json:"revision"`
	Rules         []Rule `json:"rules"`
}

// ValidScopeID checks bounded opaque Bot/group/user identifiers, not filesystem paths.
func ValidScopeID(id string, maxBytes int) bool {
	if id == "" || len(id) > maxBytes || !utf8.ValidString(id) {
		return false
	}
	for _, r := range id {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return false
		}
	}
	return true
}

// Validate checks a complete rule. An empty string is invalid; keyword whitespace
// is intentional and is never trimmed or case-normalized.
func (r Rule) Validate() error {
	if !idPattern.MatchString(r.ID) {
		return errors.New("id: expected 1-64 letters, digits, underscores or hyphens")
	}
	if strings.TrimSpace(r.Name) == "" || len(r.Name) > 128 || !utf8.ValidString(r.Name) {
		return errors.New("name: nonblank UTF-8 text up to 128 bytes required")
	}
	if r.Priority < -1000 || r.Priority > 1000 {
		return errors.New("priority: must be between -1000 and 1000")
	}
	switch r.Trigger.Type {
	case "exact", "contains", "prefix":
	default:
		return errors.New("trigger.type: supported values are exact, contains, prefix")
	}
	if len(r.Trigger.Value) == 0 || len(r.Trigger.Value) > MaxKeywordBytes || !utf8.ValidString(r.Trigger.Value) {
		return errors.New("trigger.value: nonempty UTF-8 keyword up to 1024 bytes required")
	}
	if len(r.Conditions.Sources) == 0 || len(r.Conditions.Sources) > 2 {
		return errors.New("conditions.sources: select group and/or c2c")
	}
	seen := map[events.Source]bool{}
	for _, source := range r.Conditions.Sources {
		if (source != events.Group && source != events.C2C) || seen[source] {
			return errors.New("conditions.sources: invalid or duplicate source")
		}
		seen[source] = true
	}
	if len(r.Conditions.GroupIDs) > MaxGroupIDs {
		return errors.New("conditions.group_ids: at most 32 IDs allowed")
	}
	groups := map[string]bool{}
	for _, id := range r.Conditions.GroupIDs {
		if !ValidScopeID(id, MaxIDBytes) || groups[id] {
			return errors.New("conditions.group_ids: invalid or duplicate group ID")
		}
		groups[id] = true
	}
	if r.Action.Type != "reply_text" {
		return errors.New("action.type: only reply_text is supported")
	}
	if strings.TrimSpace(r.Action.Text) == "" || len(r.Action.Text) > MaxTemplateBytes || !utf8.ValidString(r.Action.Text) {
		return errors.New("action.text: nonblank UTF-8 reply up to 4096 bytes required")
	}
	return nil
}

// Prepare validates and copies a set into deterministic descending-priority,
// ascending-ID order. It never mutates the input or performs I/O.
func Prepare(input []Rule) ([]Rule, error) {
	if len(input) > MaxRules {
		return nil, errors.New("at most 100 rules per Bot allowed")
	}
	out := make([]Rule, len(input))
	seen := map[string]bool{}
	for i, r := range input {
		if err := r.Validate(); err != nil {
			return nil, fmt.Errorf("rules[%d]: %w", i, err)
		}
		if seen[r.ID] {
			return nil, fmt.Errorf("rules[%d].id: duplicate ID", i)
		}
		seen[r.ID] = true
		r.Conditions.Sources = append([]events.Source(nil), r.Conditions.Sources...)
		r.Conditions.GroupIDs = append([]string{}, r.Conditions.GroupIDs...)
		out[i] = r
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority > out[j].Priority
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}
func cloneSet(s Set) Set {
	out := s
	out.Rules = make([]Rule, len(s.Rules))
	for i, r := range s.Rules {
		r.Conditions.Sources = append([]events.Source(nil), r.Conditions.Sources...)
		r.Conditions.GroupIDs = append([]string{}, r.Conditions.GroupIDs...)
		out.Rules[i] = r
	}
	return out
}
