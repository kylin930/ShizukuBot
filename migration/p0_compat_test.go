package migration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"shizukubot/plugins"
	"strings"
	"testing"
)

// Preserved P0 fixture and reply assertions now exercise offline conversion plus compiled Go packages.
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

type messageCase struct{ Bot, ID, Message, Source string }

// Build and execute the actual generated package; no interpreter or QQ client exists.
func runMigrated(t *testing.T, o Options, cases []messageCase) [][]string {
	t.Helper()
	o.Apply = true
	report, err := Run(o)
	if err != nil || report.Items[0].Status != "generated-go-source" {
		t.Fatal(report, err)
	}
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	module := filepath.Dir(o.Destination)
	mod := fmt.Sprintf("module migrationcheck\n\ngo 1.26\nrequire shizukubot v0.0.0\nreplace shizukubot => %s\n", filepath.ToSlash(root))
	if err = os.WriteFile(filepath.Join(module, "go.mod"), []byte(mod), 0600); err != nil {
		t.Fatal(err)
	}
	harness := `package main
import("context";"encoding/json";"os";"shizukubot/events";sdk "shizukubot/plugins";old "migrationcheck/code/old")
func main(){
 var input []struct{Bot,ID,Message,Source string};if err:=json.NewDecoder(os.Stdin).Decode(&input);err!=nil{panic(err)}
 registry:=sdk.NewRegistry();if err:=registry.Register(old.Definition());err!=nil{panic(err)}
 data,err:=os.MkdirTemp("","migrated-state-");if err!=nil{panic(err)};defer os.RemoveAll(data)
 bus:=events.NewBus();m,err:=sdk.NewManager(registry,bus,data);if err!=nil{panic(err)};defer m.Close(context.Background())
 bots:=map[string]bool{};for _,c:=range input{if !bots[c.Bot]{if err=m.Enable(context.Background(),"old",c.Bot);err!=nil{panic(err)};bots[c.Bot]=true}}
 dispatcher:=events.NewDispatcher(bus);out:=[][]string{}
 for _,c:=range input{
  replies:=[]string{};typ:=events.Type("c2c_message");group:="";if c.Source=="group"{typ="group_message";group="g"}
  err=dispatcher.Dispatch(context.Background(),events.Event{BotID:c.Bot,Type:typ,Source:events.Source(c.Source),UserID:"u",GroupID:group,MessageID:c.ID,Message:c.Message,Reply:func(_ context.Context,r events.Reply)error{replies=append(replies,r.Text);return nil}})
  if err!=nil{panic(err)};out=append(out,replies)
 }
 if err=json.NewEncoder(os.Stdout).Encode(out);err!=nil{panic(err)}
}`
	if err = os.WriteFile(filepath.Join(module, "main.go"), []byte(harness), 0600); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(module, "check")
	build := exec.Command("go", "build", "-buildvcs=false", "-mod=mod", "-o", binary, ".")
	build.Dir = module
	if raw, err := build.CombinedOutput(); err != nil {
		t.Fatalf("generated Go package does not compile: %v\n%s", err, raw)
	}
	input, _ := json.Marshal(cases)
	run := exec.Command(binary)
	run.Stdin = bytes.NewReader(input)
	raw, err := run.CombinedOutput()
	if err != nil {
		t.Fatalf("generated plugin failed: %v %s", err, raw)
	}
	var replies [][]string
	if err = json.Unmarshal(raw, &replies); err != nil {
		t.Fatal(err, string(raw))
	}
	return replies
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
	cases := []messageCase{}
	for _, source := range []string{"c2c", "group"} {
		for _, message := range []string{"/ping", " /ping args ", "/ping \"unfinished", "<@ABC> /ping"} {
			cases = append(cases, messageCase{Bot: "chosen", Message: message, Source: source})
		}
	}
	for _, reply := range runMigrated(t, o, cases) {
		if len(reply) != 1 || reply[0] != "pong" {
			t.Fatal(reply)
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
	cases := []messageCase{{"a", "one", "/ping", "group"}, {"a", "one", "/ping", "group"}, {"a", "two", "/ping", "group"}, {"b", "one", "/ping", "group"}}
	count := 0
	for _, reply := range runMigrated(t, o, cases) {
		count += len(reply)
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
	os.WriteFile(filepath.Join(o.Destination, "old", "plugin.go"), []byte("// user work"), 0600)
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
	replies := runMigrated(t, o, []messageCase{{Bot: "a", Message: "/ping", Source: "c2c"}})
	if len(replies) != 1 || len(replies[0]) != 1 || replies[0][0] != unicode {
		t.Fatal(replies)
	}
}

func TestConditionalReplyComparison(t *testing.T) {
	g := `{"nodes":[{"id":1,"type":"event/on_c2c_message","outputs":[{"name":"触发","type":-1,"links":[1]}]},{"id":2,"type":"condition/text_contains","properties":{"keyword":"hello"},"inputs":[{"name":"触发","type":-1,"link":1}],"outputs":[{"name":"结果","type":"boolean","links":[2]}]},{"id":3,"type":"action/send_message","properties":{"template":"matched"},"inputs":[{"name":"触发","type":-1,"link":2}]}],"links":[[1,1,0,2,0,-1],[2,2,0,3,0,-1]]}`
	o, _ := fixture(t, g)
	replies := runMigrated(t, o, []messageCase{{Bot: "a", Message: "hello there", Source: "c2c"}, {Bot: "a", Message: "no match", Source: "c2c"}})
	if len(replies) != 2 || len(replies[0]) != 1 || replies[0][0] != "matched" || len(replies[1]) != 0 {
		t.Fatal(replies)
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

func TestRetiredScriptsAreBackedUpAndRequireGoRewrite(t *testing.T) {
	o, _ := fixture(t, replyGraph)
	o.ScriptSource = filepath.Join(t.TempDir(), "scripts")
	o.Apply = true
	dir := filepath.Join(o.ScriptSource, "old-script")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	raw := []byte("// Retired source must be preserved, never executed.\nthrow new Error('do not run');\n")
	if err := os.WriteFile(filepath.Join(dir, "index.js"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	r, err := Run(o)
	if err != nil || len(r.Scripts) != 1 || r.Scripts[0].Status != "manual-go-rewrite" {
		t.Fatal(r, err)
	}
	backup, err := os.ReadFile(filepath.Join(r.Backup, "script_plugins", "old-script", "index.js"))
	if err != nil || string(backup) != string(raw) {
		t.Fatal("script backup mismatch", err)
	}
	original, err := os.ReadFile(filepath.Join(dir, "index.js"))
	if err != nil || string(original) != string(raw) {
		t.Fatal("script source changed", err)
	}
	if _, err = os.Stat(filepath.Join(o.Destination, "old-script")); !os.IsNotExist(err) {
		t.Fatal("script was given a fake Go translation")
	}
}
