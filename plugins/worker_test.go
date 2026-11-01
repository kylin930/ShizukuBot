package plugins

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"shizukubot/events"
	"strconv"
	"strings"
	"testing"
	"time"
)

func bundle(t *testing.T, id, code string, permissions []string) []byte {
	t.Helper()
	mf := Manifest{APIVersion: 1, ID: id, Name: id, Version: "1.0.0", Runtime: "javascript", Entry: "dist/index.js", Permissions: permissions, ConfigSchema: Schema{Type: "object"}}
	data, _ := json.Marshal(mf)
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	for name, data := range map[string][]byte{"manifest.json": data, "dist/index.js": []byte(code)} {
		w, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = w.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func manager(t *testing.T) *Manager {
	t.Helper()
	s, err := NewStore(filepath.Join(t.TempDir(), "packages"))
	if err != nil {
		t.Fatal(err)
	}
	m, err := NewManager(s, events.NewBus(), t.TempDir())
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
func publish(t *testing.T, m *Manager, bot, message string) ([]string, error) {
	t.Helper()
	var replies []string
	e := events.Event{BotID: bot, Type: "c2c_message", Source: events.C2C, UserID: "u", Message: message, Reply: func(_ context.Context, r events.Reply) error { replies = append(replies, r.Text); return nil }}
	err := m.Bus.Publish(context.Background(), e)
	return replies, err
}
func TestWorkerCommandsAndBotIsolation(t *testing.T) {
	m := manager(t)
	code := `import {definePlugin} from '@shizukubot/sdk';let n=0;export default definePlugin({id:'fixture',version:'1.0.0',commands:{ping:async c=>{await c.reply(c.botId+':'+(++n)+':'+c.args.join('|'));}}});`
	if _, err := m.Store.Install(bundle(t, "fixture", code, []string{"commands.register", "message.send"})); err != nil {
		t.Fatal(err)
	}
	if m.Enable(context.Background(), "fixture", "a", false) == nil {
		t.Fatal("untrusted autoexecution")
	}
	for _, bot := range []string{"a", "b"} {
		if err := m.Enable(context.Background(), "fixture", bot, true); err != nil {
			t.Fatal(err)
		}
	}
	m.Enable(context.Background(), "fixture", "a", false)
	for _, c := range []struct{ bot, message, want string }{{"a", `/ping "two words"`, "a:1:two words"}, {"b", "/ping", "b:1:"}, {"a", "/ping", "a:2:"}} {
		r, err := publish(t, m, c.bot, c.message)
		if err != nil || len(r) != 1 || r[0] != c.want {
			t.Fatal(r, err)
		}
	}
	if err := m.Disable(context.Background(), "fixture", "a"); err != nil {
		t.Fatal(err)
	}
	r, err := publish(t, m, "a", "/ping")
	if err != nil || len(r) != 0 {
		t.Fatal(r, err)
	}
	if err = m.Enable(context.Background(), "fixture", "a", false); err != nil {
		t.Fatal(err)
	}
	r, err = publish(t, m, "a", "/ping")
	if err != nil || len(r) != 1 || r[0] != "a:1:" {
		t.Fatal("duplicate subscription or process state", r, err)
	}
}
func TestWorkerCrashRecoveryTimeoutAndOtherSubscribers(t *testing.T) {
	m := manager(t)
	code := `export default {id:'fault',version:'1.0.0',commands:{crash:()=>process.exit(9),hang:()=>{while(true){}},ping:async c=>c.reply('recovered')}};`
	m.Store.Install(bundle(t, "fault", code, []string{"commands.register", "message.send"}))
	if err := m.Enable(context.Background(), "fault", "a", true); err != nil {
		t.Fatal(err)
	}
	other := 0
	m.Bus.Subscribe(events.Filter{BotID: "a"}, func(context.Context, events.Event) error { other++; return nil })
	if _, err := publish(t, m, "a", "/crash"); err == nil {
		t.Fatal("worker exit hidden")
	}
	time.Sleep(120 * time.Millisecond)
	r, err := publish(t, m, "a", "/ping")
	if err != nil || len(r) != 1 || r[0] != "recovered" {
		t.Fatal(r, err)
	}
	m.Timeout = 100 * time.Millisecond
	start := time.Now()
	if _, err = publish(t, m, "a", "/hang"); err == nil {
		t.Fatal("infinite code not terminated")
	}
	if time.Since(start) > time.Second || other != 3 {
		t.Fatal("timeout blocked later subscriber", other)
	}
}
func TestWorkerPermissionAndScrubbedEnvironment(t *testing.T) {
	t.Setenv("QQ_APP_SECRET", "never-forward-this")
	m := manager(t)
	code := `export default {id:'guard',version:'1.0.0',commands:{ping:async c=>{if(process.env.QQ_APP_SECRET)throw Error('credential leaked');await c.reply('forbidden');}}};`
	m.Store.Install(bundle(t, "guard", code, []string{"commands.register"}))
	if err := m.Enable(context.Background(), "guard", "a", true); err != nil {
		t.Fatal(err)
	}
	r, err := publish(t, m, "a", "/ping")
	if err == nil || !strings.Contains(err.Error(), "denied") || len(r) != 0 {
		t.Fatal(r, err)
	}
}
func TestUnsafePackagesAndAtomicInstall(t *testing.T) {
	m := manager(t)
	data := bundle(t, "valid", `export default {id:'valid',version:'1.0.0'};`, nil)
	if _, err := m.Store.Install(data); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Store.Install(data); err == nil {
		t.Fatal("overwrote installed package")
	}
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	w, _ := z.Create("../escape")
	w.Write([]byte("bad"))
	z.Close()
	if _, err := m.Store.Install(b.Bytes()); err == nil {
		t.Fatal("path traversal")
	}
	if _, err := m.Store.Get("valid"); err != nil {
		t.Fatal("failed install damaged package", err)
	}
	archive, err := m.Store.Export("valid")
	if err != nil {
		t.Fatal(err)
	}
	other, _ := NewStore(t.TempDir())
	if _, err = other.Install(archive); err != nil {
		t.Fatal("package restore", err)
	}
}
func TestBuiltTypeScriptExamples(t *testing.T) {
	m := manager(t)
	for _, id := range []string{"hello", "keywords", "counter"} {
		dir := filepath.Join("..", "examples", "plugins", id)
		var b bytes.Buffer
		z := zip.NewWriter(&b)
		for _, file := range []string{"manifest.json", "dist/index.js", "package.json"} {
			raw, err := os.ReadFile(filepath.Join(dir, file))
			if err != nil {
				t.Fatal("build examples with npm run build:examples:", err)
			}
			w, _ := z.Create(file)
			w.Write(raw)
		}
		z.Close()
		if _, err := m.Store.Install(b.Bytes()); err != nil {
			t.Fatal(err)
		}
		if err := m.Enable(context.Background(), id, "bot", true); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct{ message, want string }{{"/ping", "Pong!"}, {"hello world", "Hello!"}, {"/count", "Count: 1"}, {"/count", "Count: 2"}} {
		r, err := publish(t, m, "bot", tc.message)
		if err != nil || len(r) != 1 || r[0] != tc.want {
			t.Fatal(r, err)
		}
	}
	if err := m.Restart(context.Background(), "counter", "bot"); err != nil {
		t.Fatal(err)
	}
	r, err := publish(t, m, "bot", "/count")
	if err != nil || len(r) != 1 || r[0] != "Count: 3" {
		t.Fatal("state persistence", r, err)
	}
}

func TestProtocolRejectsForgedVersionAndID(t *testing.T) {
	for _, frame := range []string{`{"v":2,"id":"1","type":"result","payload":{}}`, `{"v":1,"id":"wrong","type":"result","payload":{}}`} {
		t.Run(frame, func(t *testing.T) {
			m := manager(t)
			code := `process.stdout.write(` + strconv.Quote(frame+"\n") + `);export default {id:'badwire',version:'1.0.0'};`
			if _, err := m.Store.Install(bundle(t, "badwire", code, nil)); err != nil {
				t.Fatal(err)
			}
			if m.Enable(context.Background(), "badwire", "a", true) == nil {
				t.Fatal("invalid IPC accepted")
			}
		})
	}
}
func TestGoRegistryInstancesAndCleanup(t *testing.T) {
	m := manager(t)
	var all []*extension
	manifest := Manifest{APIVersion: 1, ID: "native", Name: "native", Version: "1.0.0", Runtime: "go", Permissions: []string{"commands.register"}, ConfigSchema: Schema{Type: "object"}}
	if err := m.RegisterGo(manifest, func() Extension { e := &extension{}; all = append(all, e); return e }); err != nil {
		t.Fatal(err)
	}
	for _, bot := range []string{"a", "b"} {
		if err := m.Enable(context.Background(), "native", bot, true); err != nil {
			t.Fatal(err)
		}
	}
	publish(t, m, "a", "/ping")
	if len(all) != 2 || all[0].calls != 1 || all[1].calls != 0 {
		t.Fatal("Go instance state crossed bots")
	}
	if err := m.Disable(context.Background(), "native", "a"); err != nil {
		t.Fatal(err)
	}
	if all[0].stop != 1 || all[0].unload != 1 {
		t.Fatal("Go extension not released")
	}
}
