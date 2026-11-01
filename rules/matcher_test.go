package rules

import (
	"context"
	"errors"
	"shizukubot/events"
	"strings"
	"testing"
)

func sample(id string) Rule {
	return Rule{ID: id, Name: id, Enabled: true, Priority: 100, Trigger: Trigger{Type: "contains", Value: "你好"}, Conditions: Conditions{Sources: []events.Source{events.Group, events.C2C}, GroupIDs: []string{}}, Action: Action{Type: "reply_text", Text: "你好！"}, StopOnMatch: true}
}
func message() events.Event {
	return events.Event{BotID: "a", Type: "group_message", Source: events.Group, GroupID: "g", UserID: "u", Message: "你好"}
}
func TestLiteralMatchersPreserveCaseAndWhitespace(t *testing.T) {
	for _, tc := range []struct {
		kind, keyword, input string
		want                 bool
	}{{"exact", "Menu", "Menu", true}, {"exact", "Menu", "menu", false}, {"exact", "菜单", " 菜单", false}, {"contains", "你好", "你好！", true}, {"prefix", "天气 ", "天气 北京", true}, {"prefix", "天气 ", "天气", false}, {"exact", " ", " ", true}} {
		rule := sample("r")
		rule.Trigger = Trigger{Type: tc.kind, Value: tc.keyword}
		event := message()
		event.Message = tc.input
		if got := Match(rule, event).Matched; got != tc.want {
			t.Fatal(tc, got)
		}
	}
}
func TestRuleValidationAndStableOrder(t *testing.T) {
	cases := []func(*Rule){func(r *Rule) { r.ID = "../escape" }, func(r *Rule) { r.Name = "" }, func(r *Rule) { r.Trigger.Value = "" }, func(r *Rule) { r.Trigger.Type = "regex" }, func(r *Rule) { r.Action.Text = " " }, func(r *Rule) { r.Action.Type = "shell" }, func(r *Rule) { r.Conditions.Sources = []events.Source{"guild"} }, func(r *Rule) { r.Conditions.GroupIDs = []string{"bad id"} }, func(r *Rule) { r.Priority = 1001 }, func(r *Rule) { r.Trigger.Value = strings.Repeat("a", MaxKeywordBytes+1) }}
	for i, change := range cases {
		r := sample("r")
		change(&r)
		if r.Validate() == nil {
			t.Fatal("invalid accepted", i)
		}
	}
	a, b, c := sample("z"), sample("b"), sample("a")
	a.Priority = 101
	sorted, err := Prepare([]Rule{b, c, a})
	if err != nil || sorted[0].ID != "z" || sorted[1].ID != "a" || sorted[2].ID != "b" {
		t.Fatal(sorted, err)
	}
	if _, err = Prepare([]Rule{a, a}); err == nil {
		t.Fatal("duplicate ID")
	}
	input := []Rule{sample("copy")}
	out, _ := Prepare(input)
	out[0].Conditions.GroupIDs = append(out[0].Conditions.GroupIDs, "changed")
	if len(input[0].Conditions.GroupIDs) != 0 {
		t.Fatal("mutated source")
	}
}
func TestSourcesAndGroupConditions(t *testing.T) {
	rule := sample("r")
	rule.Conditions.GroupIDs = []string{"allowed"}
	for _, tc := range []struct {
		typ    string
		source events.Source
		group  string
		want   bool
	}{{"group_message", events.Group, "allowed", true}, {"group_at_message", events.Group, "allowed", true}, {"group_message", events.Group, "other", false}, {"c2c_message", events.C2C, "", true}, {"guild_message", events.Guild, "allowed", false}, {"group_message", events.C2C, "allowed", false}} {
		e := message()
		e.Type = events.Type(tc.typ)
		e.Source = tc.source
		e.GroupID = tc.group
		if Match(rule, e).Matched != tc.want {
			t.Fatal(tc)
		}
	}
	rule.Conditions.Sources = []events.Source{events.Group}
	e := message()
	e.Type = "c2c_message"
	e.Source = events.C2C
	if Match(rule, e).Matched {
		t.Fatal("source ignored")
	}
}
func TestStopFailureAndReplyLimit(t *testing.T) {
	set := Set{Rules: []Rule{sample("a"), sample("b"), sample("c"), sample("d")}}
	sent := 0
	send := func(context.Context, events.Reply) error { sent++; return nil }
	out, err := evaluate(context.Background(), set, message(), false, send)
	if err != nil || sent != 1 || out.Traces[1].Status != "skipped_stop" {
		t.Fatal(out, err)
	}
	for i := range set.Rules {
		set.Rules[i].StopOnMatch = false
	}
	sent = 0
	out, err = evaluate(context.Background(), set, message(), false, send)
	if err != nil || sent != 3 || out.Traces[3].Status != "limited" {
		t.Fatal(out, err)
	}
	set.Rules[0].StopOnMatch = true
	sent = 0
	out, err = evaluate(context.Background(), set, message(), false, func(context.Context, events.Reply) error {
		sent++
		if sent == 1 {
			return errors.New("failure")
		}
		return nil
	})
	if !errors.Is(err, ErrReplyFailed) || sent != 3 || out.Traces[0].Status != "send_failed" || out.Traces[1].Status != "replied" {
		t.Fatal(out, err)
	}
	out, err = evaluate(context.Background(), Set{Rules: []Rule{sample("no-sender")}}, message(), false, nil)
	if !errors.Is(err, ErrReplyFailed) || out.Traces[0].Reason != "reply_unavailable" {
		t.Fatal(out, err)
	}
}
func TestPlaceholderLimitsAndSinglePass(t *testing.T) {
	e := message()
	e.Message = "{BotID}"
	text, err := expand("{用户ID}|{群ID}|{BotID}|{消息内容}|{unknown}", e)
	if err != nil || text != "u|g|a|{BotID}|{unknown}" {
		t.Fatal(text, err)
	}
	e.Message = strings.Repeat("x", MaxReplyBytes)
	if _, err = expand("prefix{消息内容}", e); err == nil {
		t.Fatal("expanded response unlimited")
	}
	r := sample("r")
	r.Action.Text = "{消息内容}"
	e.Message = strings.Repeat("你", MaxMessageBytes)
	out, err := evaluate(context.Background(), Set{Rules: []Rule{r}}, e, false, nil)
	if err == nil || out.SkipReason != "message_limit" {
		t.Fatal(out, err)
	}
}

func TestQuotasAndEmptyExpansion(t *testing.T) {
	for _, change := range []func(*Rule){
		func(r *Rule) { r.Action.Text = strings.Repeat("x", MaxTemplateBytes+1) },
		func(r *Rule) { r.Conditions.GroupIDs = make([]string, MaxGroupIDs+1) },
		func(r *Rule) { r.Conditions.Sources = []events.Source{events.Group, events.Group} },
		func(r *Rule) { r.Conditions.GroupIDs = []string{"same", "same"} },
		func(r *Rule) { r.Name = strings.Repeat("x", 129) },
		func(r *Rule) { r.ID = strings.Repeat("x", 65) },
	} {
		r := sample("r")
		change(&r)
		if r.Validate() == nil {
			t.Fatal("quota accepted", r.ID)
		}
	}
	if _, err := Prepare(make([]Rule, MaxRules+1)); err == nil {
		t.Fatal("rule count unbounded")
	}
	e := message()
	e.GroupID = ""
	if _, err := expand("{群ID}", e); err == nil || err.Error() != "expanded_reply_empty" {
		t.Fatal(err)
	}
	e.Message = strings.Repeat("x", MaxMessageBytes)
	if _, err := expand(strings.Repeat("{消息内容}", 100), e); err == nil {
		t.Fatal("unbounded expansion")
	}
	r := sample("a")
	r.Action.Text = "{群ID}"
	other := sample("b")
	out, err := evaluate(context.Background(), Set{Rules: []Rule{r, other}}, e, false, func(context.Context, events.Reply) error { return nil })
	if err != nil || out.Traces[0].Status != "not_matched" {
		t.Fatal(out, err)
	}
	e.Message = "你好"
	out, err = evaluate(context.Background(), Set{Rules: []Rule{r, other}}, e, false, func(context.Context, events.Reply) error { return nil })
	if err != nil || out.Traces[0].Reason != "expanded_reply_empty" || len(out.Replies) != 1 {
		t.Fatal(out, err)
	}
}
