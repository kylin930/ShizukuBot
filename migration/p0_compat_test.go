package migration

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"shizukubot/events"
	"shizukubot/plugins"
	"strings"
	"testing"
	"time"
)

// Preserved P0 fixture and reply assertions now exercise offline conversion plus the new worker.
const replyGraph = `{"nodes":[{"id":1,"type":"event/on_command","properties":{"command":"ping","prefix":"/"},"outputs":[{"name":"触发","type":-1,"links":[1]}]},{"id":2,"type":"action/send_message","properties":{"template":"pong"},"inputs":[{"name":"触发","type":-1,"link":1}]}],"links":[[1,1,0,2,0,-1]]}`

func fixture(t *testing.T, graph string) (Options, []byte) {
	t.Helper()
	root := t.TempDir()
	o := Options{Source: filepath.Join(root, "legacy"), CustomNodes: filepath.Join(root, "custom"), Destination: filepath.Join(root, "code"), Backups: filepath.Join(root, "backups")}
	os.MkdirAll(o.Source, 0700)
	os.MkdirAll(o.CustomNodes, 0700)
	b, _ := json.Marshal(map[string]any{"id": "old", "name": "old", "enabled": true, "bot_id": "chosen", "graph": json.RawMessage(graph)})
	if err := os.WriteFile(filepath.Join(o.Source, "old.json"), b, 0600); err != nil {
		t.Fatal(err)
	}
	return o, b
}
func migrated(t *testing.T, o Options) *plugins.Manager {
	t.Helper()
	o.Apply = true
	report, err := Run(o)
	if err != nil || len(report.Items) != 1 || report.Items[0].Status != "converted-disabled" {
		t.Fatal(report, err)
	}
	s, err := plugins.NewStore(o.Destination)
	if err != nil {
		t.Fatal(err)
	}
	m, err := plugins.NewManager(s, events.NewBus(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := m.Close(ctx); err != nil {
			t.Error(err)
		}
	})
	return m
}
func send(m *plugins.Manager, message, bot string, source events.Source, id string) ([]string, error) {
	replies := []string{}
	typ := events.Type("c2c_message")
	group := ""
	if source == events.Group {
		typ = "group_message"
		group = "g"
	}
	e := events.Event{BotID: bot, Type: typ, Source: source, UserID: "u", GroupID: group, MessageID: id, Message: message, Reply: func(_ context.Context, r events.Reply) error { replies = append(replies, r.Text); return nil }}
	d := events.NewDispatcher(m.Bus)
	err := d.Dispatch(context.Background(), e)
	return replies, err
}
func TestLegacyLoadAndRepliesAllSources(t *testing.T) {
	o, original := fixture(t, replyGraph)
	dry, err := Run(o)
	if err != nil || dry.Items[0].Status != "convertible" {
		t.Fatal(dry, err)
	}
	if _, err = os.Stat(o.Destination); !os.IsNotExist(err) {
		t.Fatal("dry run wrote files")
	}
	m := migrated(t, o)
	if m.Enable(context.Background(), "old", "chosen", false) == nil {
		t.Fatal("migration enabled untrusted code")
	}
	if err = m.Enable(context.Background(), "old", "chosen", true); err != nil {
		t.Fatal(err)
	}
	for _, source := range []events.Source{events.C2C, events.Group} {
		for _, message := range []string{"/ping", " /ping args ", "/ping \"unfinished", "<@ABC> /ping"} {
			r, err := send(m, message, "chosen", source, "")
			if err != nil || len(r) != 1 || r[0] != "pong" {
				t.Fatal(message, r, err)
			}
		}
	}
	b, _ := os.ReadFile(filepath.Join(o.Source, "old.json"))
	if string(b) != string(original) {
		t.Fatal("original plugin changed")
	}
}
func TestLegacyErrorAndPanicDoNotStopOtherPlugins(t *testing.T) {
	for _, kind := range []string{"code/go", "logic/branch", "test/panic"} {
		o, _ := fixture(t, strings.Replace(replyGraph, "action/send_message", kind, 1))
		o.Apply = true
		r, err := Run(o)
		if err != nil || r.Items[0].Status != "manual" || r.Items[0].Reason == "" {
			t.Fatal(r, err)
		}
		if _, err = os.Stat(filepath.Join(o.Destination, "old")); !os.IsNotExist(err) {
			t.Fatal("unsupported graph produced executable fallback")
		}
	}
}
func TestLegacyEngineDedupUsesIDs(t *testing.T) {
	o, _ := fixture(t, replyGraph)
	m := migrated(t, o)
	for _, bot := range []string{"a", "b"} {
		if err := m.Enable(context.Background(), "old", bot, true); err != nil {
			t.Fatal(err)
		}
	}
	d := events.NewDispatcher(m.Bus)
	count := 0
	for _, tc := range []struct{ bot, id string }{{"a", "one"}, {"a", "one"}, {"a", "two"}, {"b", "one"}} {
		e := events.Event{BotID: tc.bot, Type: "group_message", Source: events.Group, GroupID: "g", UserID: "u", MessageID: tc.id, Message: "/ping", Reply: func(context.Context, events.Reply) error { count++; return nil }}
		if err := d.Dispatch(context.Background(), e); err != nil {
			t.Fatal(err)
		}
	}
	if count != 3 {
		t.Fatal("dedup or bot isolation", count)
	}
}
func TestBackupRepeatAndRollback(t *testing.T) {
	o, b := fixture(t, replyGraph)
	os.WriteFile(filepath.Join(o.CustomNodes, "custom.json"), []byte(`{"operations":[]}`), 0600)
	o.Apply = true
	r, err := Run(o)
	if err != nil {
		t.Fatal(err)
	}
	backup, _ := os.ReadFile(filepath.Join(r.Backup, "plugins", "old.json"))
	if string(backup) != string(b) {
		t.Fatal("inexact backup")
	}
	if _, err = os.Stat(filepath.Join(r.Backup, "custom_nodes", "custom.json")); err != nil {
		t.Fatal("missing custom node backup")
	}
	again, err := Run(o)
	if err != nil || again.Items[0].Status != "already-migrated" {
		t.Fatal(again, err)
	}
	if err = Rollback(r.Backup, o.Destination); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(o.Destination, "old")); !os.IsNotExist(err) {
		t.Fatal("rollback did not remove generated project")
	}
	if _, err = os.Stat(filepath.Join(o.Source, "old.json")); err != nil {
		t.Fatal("rollback lost original")
	}
}
func TestRollbackPreservesUserEdits(t *testing.T) {
	o, _ := fixture(t, replyGraph)
	o.Apply = true
	r, err := Run(o)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(o.Destination, "old", "dist", "index.js"), []byte("// user work"), 0600)
	if Rollback(r.Backup, o.Destination) == nil {
		t.Fatal("deleted edited plugin")
	}
}

func TestGlobalCustomOverrideRequiresManualReview(t *testing.T) {
	o, _ := fixture(t, replyGraph)
	os.WriteFile(filepath.Join(o.CustomNodes, "override.json"), []byte(`{"type":"action/send_message","operations":[]}`), 0600)
	r, err := Run(o)
	if err != nil || r.Items[0].Status != "manual" {
		t.Fatal("overridden capability falsely migrated", r, err)
	}
}
func TestPendingJournalCanRollback(t *testing.T) {
	o, _ := fixture(t, replyGraph)
	o.Apply = true
	r, err := Run(o)
	if err != nil {
		t.Fatal(err)
	}
	r.Items[0].Status = "conversion-pending"
	if err = plugins.AtomicJSON(filepath.Join(r.Backup, "report.json"), r); err != nil {
		t.Fatal(err)
	}
	if err = Rollback(r.Backup, o.Destination); err != nil {
		t.Fatal(err)
	}
}

func TestUnicodeLiteralReply(t *testing.T) {
	unicode := "Hello 😀\u2028next"
	graph := strings.Replace(replyGraph, "pong", unicode, 1)
	o, _ := fixture(t, graph)
	m := migrated(t, o)
	if err := m.Enable(context.Background(), "old", "a", true); err != nil {
		t.Fatal(err)
	}
	r, err := send(m, "/ping", "a", events.C2C, "")
	if err != nil || len(r) != 1 || r[0] != unicode {
		t.Fatal(r, err)
	}
}

func TestConditionalReplyComparison(t *testing.T) {
	g := `{"nodes":[{"id":1,"type":"event/on_c2c_message","outputs":[{"name":"触发","type":-1,"links":[1]}]},{"id":2,"type":"condition/text_contains","properties":{"keyword":"hello"},"inputs":[{"name":"触发","type":-1,"link":1}],"outputs":[{"name":"结果","type":"boolean","links":[2]}]},{"id":3,"type":"action/send_message","properties":{"template":"matched"},"inputs":[{"name":"触发","type":-1,"link":2}]}],"links":[[1,1,0,2,0,-1],[2,2,0,3,0,-1]]}`
	o, _ := fixture(t, g)
	m := migrated(t, o)
	if err := m.Enable(context.Background(), "old", "a", true); err != nil {
		t.Fatal(err)
	}
	for _, message := range []string{"hello there", "no match"} {
		r, err := send(m, message, "a", events.C2C, "")
		expected := 0
		if strings.Contains(message, "hello") {
			expected = 1
		}
		if err != nil || len(r) != expected {
			t.Fatal(message, r, err)
		}
	}
}

func TestEventDataPortIsNotAssumedToBeTrigger(t *testing.T) {
	g := strings.Replace(replyGraph, `"name":"触发","type":-1,"links":[1]`, `"name":"消息文本","type":"string","links":[1]`, 1)
	o, _ := fixture(t, g)
	r, err := Run(o)
	if err != nil || r.Items[0].Status != "manual" {
		t.Fatal("data-driven trigger falsely migrated", r, err)
	}
}
