package plugins

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
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
	t.Setenv("NODE_OPTIONS", "--require=/this/path/must/not/load.cjs")
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

func TestBackupRestoreIsDisabledAndPreservesState(t *testing.T) {
	m := manager(t)
	code := `export default {id:'stateful',version:'1.0.0',commands:{ping:async c=>{let n=(await c.state.get('n')??0)+1;await c.state.set('n',n);await c.reply(String(n))}}};`
	_, err := m.InstallPackage(bundle(t, "stateful", code, []string{"commands.register", "message.send", "state.read", "state.write"}), false)
	if err != nil {
		t.Fatal(err)
	}
	if err = m.Enable(context.Background(), "stateful", "a", true); err != nil {
		t.Fatal(err)
	}
	publish(t, m, "a", "/ping")
	data, err := m.ExportBackup("stateful")
	if err != nil {
		t.Fatal(err)
	}
	next := manager(t)
	if _, err = next.InstallPackage(data, true); err != nil {
		t.Fatal(err)
	}
	s := next.Config("stateful", "a")
	if s.Enabled || s.Trusted {
		t.Fatal("backup auto-enabled trusted code")
	}
	if err = next.Enable(context.Background(), "stateful", "a", true); err != nil {
		t.Fatal(err)
	}
	r, err := publish(t, next, "a", "/ping")
	if err != nil || len(r) != 1 || r[0] != "2" {
		t.Fatal("lost restored state", r, err)
	}
	if _, err = next.InstallPackage(data, true); err == nil {
		t.Fatal("backup overwrote existing data")
	}
}
func TestDisableCancelsInflightWorker(t *testing.T) {
	m := manager(t)
	code := `export default {id:'cancel',version:'1.0.0',commands:{wait:async c=>{await c.log('entered');await new Promise(()=>{})}}};`
	m.Store.Install(bundle(t, "cancel", code, []string{"commands.register", "log.write"}))
	entered := make(chan struct{}, 1)
	m.Log = func(_, _, msg string) {
		if msg == "entered" {
			entered <- struct{}{}
		}
	}
	if err := m.Enable(context.Background(), "cancel", "a", true); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { defer close(done); publish(t, m, "a", "/wait") }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("worker did not enter")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := m.Disable(ctx, "cancel", "a"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("inflight worker leaked")
	}
}

func TestMultiplePluginsAndConfigurationIsolation(t *testing.T) {
	m := manager(t)
	for _, id := range []string{"first", "second"} {
		code := `export default {id:` + strconv.Quote(id) + `,version:'1.0.0',events:{c2c_message:async c=>c.reply(` + strconv.Quote(id) + `)}};`
		if _, err := m.Store.Install(bundle(t, id, code, []string{"events.subscribe", "message.send"})); err != nil {
			t.Fatal(err)
		}
		if err := m.Enable(context.Background(), id, "a", true); err != nil {
			t.Fatal(err)
		}
	}
	r, err := publish(t, m, "a", "ordinary message")
	if err != nil || len(r) != 2 || r[0] != "first" || r[1] != "second" {
		t.Fatal(r, err)
	}
	if err = m.Disable(context.Background(), "first", "a"); err != nil {
		t.Fatal(err)
	}
	r, err = publish(t, m, "a", "ordinary message")
	if err != nil || len(r) != 1 || r[0] != "second" {
		t.Fatal("subscription not removed", r, err)
	}
}
func TestInvalidConfigAndPackages(t *testing.T) {
	m := manager(t)
	if _, err := m.Store.Install(bundle(t, "config", "export default {};", []string{"unknown.permission"})); err == nil {
		t.Fatal("unknown permission accepted")
	}
	if _, err := m.Store.Install(bundle(t, "config", "export default {};", nil)); err != nil {
		t.Fatal(err)
	}
	if m.Configure("config", "a", map[string]any{"unexpected": true}) == nil {
		t.Fatal("invalid config accepted")
	}
	if m.Configure("config", "a", map[string]any{"invalid": make(chan int)}) == nil {
		t.Fatal("non-JSON Go config accepted")
	}
}
func TestPortableZipValidation(t *testing.T) {
	valid := bundle(t, "portable", `export default {id:'portable',version:'1.0.0'};`, nil)
	base, err := zip.NewReader(bytes.NewReader(valid), int64(len(valid)))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"../escape", "C:\\escape", "dist/CON.js", "DIST/index.js", "common.js"} {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			writer := zip.NewWriter(&buf)
			for _, f := range base.File {
				reader, _ := f.Open()
				b, _ := io.ReadAll(reader)
				reader.Close()
				w, _ := writer.Create(f.Name)
				w.Write(b)
			}
			w, _ := writer.Create(name)
			w.Write([]byte("safe"))
			writer.Close()
			store, _ := NewStore(t.TempDir())
			_, err := store.Install(buf.Bytes())
			if name == "common.js" {
				if err != nil {
					t.Fatal("portable normal file rejected", err)
				}
			} else if err == nil {
				t.Fatal("unsafe portable path accepted")
			}
		})
	}
}

func TestCloseCancelsDebugAndPreventsNewWorkers(t *testing.T) {
	m := manager(t)
	code := `export default {id:'debugcancel',version:'1.0.0',commands:{wait:async c=>{await c.log('entered');await new Promise(()=>{})}}};`
	m.Store.Install(bundle(t, "debugcancel", code, []string{"commands.register", "log.write"}))
	m.Trust("debugcancel", "a", true)
	entered := make(chan struct{}, 1)
	m.Log = func(_, _, s string) {
		if s == "entered" {
			entered <- struct{}{}
		}
	}
	done := make(chan error, 1)
	go func() {
		done <- m.Debug(context.Background(), "debugcancel", events.Event{BotID: "a", Type: "c2c_message", Message: "/wait"}, nil)
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("debug worker did not enter")
	}
	if err := m.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancel not reported")
		}
	case <-time.After(time.Second):
		t.Fatal("debug not joined")
	}
	if m.Debug(context.Background(), "debugcancel", events.Event{BotID: "a"}, nil) == nil {
		t.Fatal("new worker after Close")
	}
}
