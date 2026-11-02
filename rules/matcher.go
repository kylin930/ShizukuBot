package rules

import (
	"context"
	"errors"
	"shizukubot/events"
	"strings"
	"unicode/utf8"
)

// Trace reports facts, not a guess based on whether a reply was present.
type Trace struct {
	RuleID         string `json:"rule_id"`
	Name           string `json:"name"`
	Priority       int    `json:"priority"`
	Enabled        bool   `json:"enabled"`
	Evaluated      bool   `json:"evaluated"`
	TriggerMatched bool   `json:"trigger_matched"`
	SourceAllowed  bool   `json:"source_allowed"`
	GroupAllowed   bool   `json:"group_allowed"`
	Matched        bool   `json:"matched"`
	Status         string `json:"status"`
	Reason         string `json:"reason,omitempty"`
	ReplyText      string `json:"reply_text,omitempty"`
}

// Result preserves evaluation order and captured/successful text replies.
type Result struct {
	BotID      string   `json:"bot_id"`
	Revision   uint64   `json:"revision"`
	Draft      bool     `json:"draft"`
	Eligible   bool     `json:"eligible"`
	SkipReason string   `json:"skip_reason,omitempty"`
	Attempts   int      `json:"attempts"`
	Replies    []string `json:"replies"`
	Traces     []Trace  `json:"traces"`
}

// Eligible accepts only the three existing canonical group/C2C message types.
func Eligible(e events.Event) bool {
	switch e.Type {
	case "group_message", "group_at_message":
		return e.Source == events.Group
	case "c2c_message":
		return e.Source == events.C2C
	}
	return false
}

// Match is pure. It does not trim messages, normalize case or dispatch events.
func Match(r Rule, e events.Event) Trace {
	t := Trace{RuleID: r.ID, Name: r.Name, Priority: r.Priority, Enabled: r.Enabled, Evaluated: true, GroupAllowed: true, Status: "not_matched"}
	for _, source := range r.Conditions.Sources {
		if source == e.Source {
			t.SourceAllowed = true
		}
	}
	if e.Source == events.Group && len(r.Conditions.GroupIDs) > 0 {
		t.GroupAllowed = false
		for _, group := range r.Conditions.GroupIDs {
			if group == e.GroupID {
				t.GroupAllowed = true
			}
		}
	}
	switch r.Trigger.Type {
	case "exact":
		t.TriggerMatched = e.Message == r.Trigger.Value
	case "contains":
		t.TriggerMatched = strings.Contains(e.Message, r.Trigger.Value)
	case "prefix":
		t.TriggerMatched = strings.HasPrefix(e.Message, r.Trigger.Value)
	}
	t.Matched = Eligible(e) && t.SourceAllowed && t.GroupAllowed && t.TriggerMatched
	if t.Matched {
		t.Status = "matched"
	}
	return t
}

func expand(text string, e events.Event) (string, error) {
	// Count expansion before allocating it. These distinct literal tokens cannot
	// overlap; inserted user text is never scanned again or interpreted as code.
	pairs := []string{"{用户ID}", e.UserID, "{群ID}", e.GroupID, "{BotID}", e.BotID, "{消息内容}", e.Message}
	size := len(text)
	for i := 0; i < len(pairs); i += 2 {
		size += strings.Count(text, pairs[i]) * (len(pairs[i+1]) - len(pairs[i]))
	}
	if size > MaxReplyBytes {
		return "", errors.New("expanded_reply_limit")
	}
	result := strings.NewReplacer(pairs...).Replace(text)
	if strings.TrimSpace(result) == "" {
		return "", errors.New("expanded_reply_empty")
	}
	return result, nil
}
func safeReply(ctx context.Context, send func(context.Context, events.Reply) error, text string) (err error) {
	defer func() {
		if recover() != nil {
			err = ErrReplyFailed
		}
	}()
	if err = ctx.Err(); err != nil {
		return err
	}
	if send == nil {
		return errors.New("reply_unavailable")
	}
	return send(ctx, events.Reply{Kind: "text", Text: text})
}
func failureCode(err error) string {
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	return "reply_failed"
}

// evaluate is shared by live execution and capture-only previews. Failure does
// not stop following matches; stop_on_match applies only after a successful send.
func evaluate(ctx context.Context, set Set, e events.Event, skipBuiltin bool, send func(context.Context, events.Reply) error) (Result, error) {
	out := Result{BotID: e.BotID, Revision: set.Revision, Eligible: Eligible(e), Replies: []string{}, Traces: []Trace{}}
	if !out.Eligible {
		out.SkipReason = "unsupported_event"
		return out, nil
	}
	if len(e.Message) > MaxMessageBytes || !utf8.ValidString(e.Message) {
		out.SkipReason = "message_limit"
		return out, errors.New("rule message exceeds input limits")
	}
	if skipBuiltin {
		out.SkipReason = "builtin_command"
	}
	stopped := false
	var errs []error
	for _, rule := range set.Rules {
		t := Trace{RuleID: rule.ID, Name: rule.Name, Priority: rule.Priority, Enabled: rule.Enabled}
		switch {
		case !rule.Enabled:
			t.Status = "disabled"
		case skipBuiltin:
			t.Status = "skipped_builtin"
		case stopped:
			t.Status = "skipped_stop"
		case ctx.Err() != nil:
			t.Status = "canceled"
			t.Reason = failureCode(ctx.Err())
		default:
			t = Match(rule, e)
			if t.Matched {
				text, err := expand(rule.Action.Text, e)
				if err != nil {
					t.Status = "limited"
					t.Reason = err.Error()
				} else if out.Attempts >= MaxReplyAttempts {
					t.Status = "limited"
					t.Reason = "reply_attempt_limit"
				} else {
					t.ReplyText = text
					out.Attempts++
					if err = safeReply(ctx, send, text); err != nil {
						t.Status = "send_failed"
						t.Reason = failureCode(err)
						if send == nil {
							t.Reason = "reply_unavailable"
						}
						errs = append(errs, ErrReplyFailed)
						if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
							errs = append(errs, err)
						}
					} else {
						t.Status = "replied"
						out.Replies = append(out.Replies, text)
						stopped = rule.StopOnMatch
					}
				}
			}
		}
		out.Traces = append(out.Traces, t)
	}
	if ctx.Err() != nil {
		errs = append(errs, ctx.Err())
	}
	return out, errors.Join(errs...)
}
