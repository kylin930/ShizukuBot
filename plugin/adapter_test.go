package plugin

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"shizukubot/events"
	"strings"
	"testing"
	"time"
)

const replyGraph = `{"nodes":[{"id":1,"type":"event/on_command","properties":{"command":"ping","prefix":"/"},"outputs":[{"name":"触发","type":-1,"links":[1]}]},{"id":2,"type":"action/send_message","properties":{"template":"pong"},"inputs":[{"name":"触发","type":-1,"link":1}]}],"links":[[1,1,0,2,0,-1]]}`

func makeLegacy(t *testing.T) (*PluginStore, *LegacyAdapter) {
	t.Helper()
	s, err := NewPluginStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	p := &Plugin{ID: "old", Name: "old", BotID: "*", Enabled: true, Graph: json.RawMessage(replyGraph)}
	if err = s.Save(p); err != nil {
		t.Fatal(err)
	}
	// Unit tests avoid creating custom-node runtime state in the source checkout.
	e := &ExecutionEngine{store: s, executors: make(map[string]NodeExecutor), dedup: events.NewDeduper(0, 0)}
	e.registerBuiltinExecutors()
	return s, &LegacyAdapter{Engine: e}
}
func TestLegacyLoadAndRepliesAllSources(t *testing.T) {
	s, a := makeLegacy(t)
	loaded, err := NewPluginStore(s.dataDir)
	if err != nil || len(loaded.List()) != 1 {
		t.Fatal("legacy JSON did not reload", err)
	}
	for _, source := range []events.Source{events.Group, events.C2C} {
		t.Run(string(source), func(t *testing.T) {
			var replies []string
			e := events.Event{BotID: "chosen", Type: "c2c_message", Source: source, Message: "/ping", UserID: "u", Reply: func(_ context.Context, r events.Reply) error { replies = append(replies, r.Text); return nil }}
			if source == events.Group {
				e.Type = "group_at_message"
				e.GroupID = "g"
			}
			if err = a.Handle(context.Background(), e); err != nil {
				t.Fatal(err)
			}
			if len(replies) != 1 || replies[0] != "pong" {
				t.Fatal(replies)
			}
		})
	}
}
func TestLegacyErrorAndPanicDoNotStopOtherPlugins(t *testing.T) {
	s, a := makeLegacy(t)
	a.Engine.RegisterExecutor("test/panic", panicExecutor{})
	for _, typ := range []string{"test/missing", "test/panic"} {
		bad := strings.Replace(replyGraph, "action/send_message", typ, 1)
		if err := s.Save(&Plugin{ID: "a-bad", Enabled: true, BotID: "*", Graph: json.RawMessage(bad)}); err != nil {
			t.Fatal(err)
		}
		count := 0
		e := events.Event{BotID: "b", Type: "c2c_message", Message: "/ping", UserID: "u", Reply: func(context.Context, events.Reply) error { count++; return nil }}
		if a.Handle(context.Background(), e) == nil {
			t.Fatal("failure hidden")
		}
		if count != 1 {
			t.Fatal("later plugin blocked")
		}
	}
}

type panicExecutor struct{}

func (panicExecutor) Execute(*EventContext, *NodeData, map[string]interface{}) (map[string]interface{}, error) {
	panic("node failure")
}
func TestPluginStoreRejectsPathsAndEscapingSymlinks(t *testing.T) {
	s, _ := makeLegacy(t)
	for _, id := range []string{"../escape", "/tmp/x", "a/b", ".."} {
		if s.Save(&Plugin{ID: id, Graph: json.RawMessage(`{}`)}) == nil || s.Delete(id) == nil {
			t.Fatal("unsafe ID", id)
		}
	}
	outside := filepath.Join(t.TempDir(), "outside.json")
	os.WriteFile(outside, []byte("preserved"), 0600)
	if err := os.Symlink(outside, filepath.Join(s.dataDir, "link.json")); err != nil {
		t.Fatal(err)
	}
	if s.Save(&Plugin{ID: "link", Graph: json.RawMessage(`{}`)}) == nil {
		t.Fatal("escaped root")
	}
	b, _ := os.ReadFile(outside)
	if string(b) != "preserved" {
		t.Fatal("external file changed")
	}
	p, _ := s.Get("old")
	p.Name = "changed"
	stored, _ := s.Get("old")
	if stored.Name == p.Name {
		t.Fatal("mutable pointer escaped")
	}
}
func TestCodeCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := (&CodeGoExecutor{}).Execute(&EventContext{Context: ctx}, &NodeData{Properties: map[string]interface{}{"code": "for {}"}}, nil)
	if err == nil {
		t.Fatal("cancellation ignored")
	}
}

func TestLegacyEngineDedupUsesIDs(t *testing.T) {
	_, a := makeLegacy(t)
	a.Engine.dedup = events.NewDeduper(time.Minute, 100)
	count := 0
	send := func(bot, id string) bool {
		return a.Engine.ProcessEvent(&EventContext{BotID: bot, EventType: "group_message", MessageID: id, GroupID: "g", UserID: "u", Message: "/ping", ReplyToGroup: func(string) error { count++; return nil }})
	}
	if !send("a", "one") || send("a", "one") || !send("a", "two") || !send("b", "one") || count != 3 {
		t.Fatalf("executions %d", count)
	}
}

func TestLegacyCaptureHelperNeverUsesLiveCallbacks(t *testing.T) {
	s, a := makeLegacy(t)
	p, _ := s.Get("old")
	p.Graph = json.RawMessage(strings.Replace(replyGraph, "action/send_message", "test/replies", 1))
	if err := s.Save(p); err != nil {
		t.Fatal(err)
	}
	a.Engine.RegisterExecutor("test/replies", allReplies{})
	live := func(string) error { t.Fatal("live reply called from capture helper"); return nil }
	replies, err := a.Engine.ProcessEventAndGetRepliesWithPlugin(&EventContext{BotID: "chosen", EventType: "c2c_message", Message: "/ping", UserID: "u", ReplyToUser: live, ReplyImageToUser: live, ReplyMarkdownToUser: live, ReplyVideoToUser: live}, "old")
	if err != nil || len(replies) != 6 {
		t.Fatalf("replies %v error %v", replies, err)
	}
}

type allReplies struct{}

func (allReplies) Execute(c *EventContext, _ *NodeData, _ map[string]interface{}) (map[string]interface{}, error) {
	for _, send := range []func() error{func() error { return c.ReplyToUser("text") }, func() error { return c.ReplyImageToUser("image") }, func() error { return c.ReplyRichMediaToUser("text", "image") }, func() error { return c.ReplyMarkdownToUser("markdown") }, func() error { return c.ReplyKeyboardToUser("markdown", "{}") }, func() error { return c.ReplyVideoToUser("video") }} {
		if err := send(); err != nil {
			return nil, err
		}
	}
	return nil, nil
}
