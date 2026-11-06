package sdk

import (
	"context"
	"encoding/json"
	"regexp"
	"testing"
)

func TestMatchHelpers(t *testing.T) {
	if rest, ok := MatchPrefix("选择 可乐 还是 雪碧", "选择"); !ok || rest != "可乐 还是 雪碧" {
		t.Fatalf("MatchPrefix = %q, %v", rest, ok)
	}
	if _, ok := MatchPrefix("不匹配", "选择"); ok {
		t.Fatal("MatchPrefix should not match")
	}
	if rest, ok := MatchSuffix("翻转 你好", "你好"); !ok || rest != "翻转" {
		t.Fatalf("MatchSuffix = %q, %v", rest, ok)
	}
	if !MatchFull("  疯狂星期四 ", "疯狂星期四") {
		t.Fatal("MatchFull should trim spaces")
	}
	re := regexp.MustCompile(`^翻转\s*([A-Za-z\s]*)$`)
	if m := MatchRegexp("翻转 abc", re); len(m) != 2 || m[1] != "abc" {
		t.Fatalf("MatchRegexp = %v", m)
	}
	if m := MatchRegexp("abc", re); m != nil {
		t.Fatalf("MatchRegexp should return nil, got %v", m)
	}
}

type memState map[string]json.RawMessage

func (m memState) Get(_ context.Context, key string) (json.RawMessage, error) { return m[key], nil }
func (m memState) Set(_ context.Context, key string, v json.RawMessage) error {
	m[key] = v
	return nil
}

func TestStateJSON(t *testing.T) {
	st := memState{}
	ctx := context.Background()
	type payload struct {
		Count int      `json:"count"`
		Names []string `json:"names"`
	}
	var got payload
	if err := LoadJSON(ctx, st, "missing", &got); err != nil {
		t.Fatalf("missing key: %v", err)
	}
	if err := SaveJSON(ctx, st, "data", payload{Count: 2, Names: []string{"a"}}); err != nil {
		t.Fatal(err)
	}
	var round payload
	if err := LoadJSON(ctx, st, "data", &round); err != nil {
		t.Fatal(err)
	}
	if round.Count != 2 || len(round.Names) != 1 || round.Names[0] != "a" {
		t.Fatalf("round trip = %+v", round)
	}
	if err := LoadJSON(ctx, nil, "data", &round); err != nil {
		t.Fatalf("nil state: %v", err)
	}
	if err := SaveJSON(ctx, st, "bad", make(chan int)); err == nil {
		t.Fatal("unmarshalable value should error")
	}
}

func TestMentionedAndText(t *testing.T) {
	e := Event{Message: "<@!123> 你好"}
	if !Mentioned(e) {
		t.Fatal("legacy mention syntax should be detected")
	}
	if got := Text(e); got != "你好" {
		t.Fatalf("Text = %q", got)
	}
	raw := json.RawMessage(`{"mentions":[{"id":"bot","is_you":true}]}`)
	e2 := Event{Message: "<@bot> 你好", Raw: raw}
	if !Mentioned(e2) {
		t.Fatal("structured mention should be detected")
	}
	if got := Text(e2); got != "你好" {
		t.Fatalf("Text with structured mention = %q", got)
	}
}
