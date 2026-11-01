package plugins

import (
	"bufio"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"shizukubot/events"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

//go:embed worker/*
var workerFiles embed.FS

// Frame is the bounded JSON-lines IPC v1 envelope. Parent links capabilities to one host request.
type Frame struct {
	V       int             `json:"v"`
	ID      string          `json:"id"`
	Parent  string          `json:"parent,omitempty"`
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
	Error   string          `json:"error,omitempty"`
}

// Worker is one Node process per plugin/Bot. It receives a scrubbed environment, never QQ credentials.
type Worker struct {
	Node, Entry, BotID string
	Log                func(string)
	Manifest           Manifest
	Config             map[string]any
	mu                 sync.Mutex
	cmd                *exec.Cmd
	input              io.WriteCloser
	inbox              chan Frame
	done               chan struct{}
	wait               chan error
	dir                string
	seq                atomic.Uint64
}

func (w *Worker) Start(ctx context.Context) (Registration, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	var reg Registration
	node := w.Node
	if node == "" {
		node = "node"
	}
	versionCommand := exec.CommandContext(ctx, node, "--version")
	versionCommand.Env = workerEnvironment()
	check, err := versionCommand.Output()
	if err != nil {
		return reg, fmt.Errorf("Node.js 24 LTS required: %w", err)
	}
	major, _ := strconv.Atoi(strings.Split(strings.TrimPrefix(strings.TrimSpace(string(check)), "v"), ".")[0])
	if major != 24 {
		return reg, errors.New("Node.js 24 LTS is required")
	}
	w.dir, err = os.MkdirTemp("", "shizuku-worker-")
	if err != nil {
		return reg, err
	}
	for _, name := range []string{"main.mjs", "bootstrap.mjs", "loader.mjs", "sdk.mjs"} {
		data, _ := workerFiles.ReadFile("worker/" + name)
		if err = os.WriteFile(filepath.Join(w.dir, name), data, 0600); err != nil {
			os.RemoveAll(w.dir)
			return reg, err
		}
	}
	w.cmd = exec.Command(node, "--max-old-space-size=96", "--import", fileModuleURL(filepath.Join(w.dir, "bootstrap.mjs")), filepath.Join(w.dir, "main.mjs"))
	w.cmd.Dir = filepath.Dir(w.Entry)
	w.cmd.Env = workerEnvironment()

	stdout, err := w.cmd.StdoutPipe()
	if err != nil {
		os.RemoveAll(w.dir)
		return reg, err
	}
	w.input, err = w.cmd.StdinPipe()
	if err != nil {
		stdout.Close()
		os.RemoveAll(w.dir)
		return reg, err
	}
	w.cmd.Stderr = &discardBounded{log: w.Log, remaining: 16384}
	if err = w.cmd.Start(); err != nil {
		os.RemoveAll(w.dir)
		return reg, err
	}
	w.inbox = make(chan Frame, 16)
	w.done = make(chan struct{})
	w.wait = make(chan error, 1)
	go func() {
		defer close(w.inbox)
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 4096), 1<<20)
		for scanner.Scan() {
			var f Frame
			if json.Unmarshal(scanner.Bytes(), &f) != nil || f.V != 1 {
				break
			}
			select {
			case w.inbox <- f:
			case <-w.done:
				return
			}
		}
	}()
	command := w.cmd
	go func() { w.wait <- command.Wait() }()
	payload := map[string]any{"id": w.Manifest.ID, "entry": w.Entry, "botId": w.BotID, "config": w.Config}
	raw, err := w.request(ctx, "load", payload, nil)
	if err == nil {
		err = json.Unmarshal(raw, &reg)
	}
	if err != nil {
		w.kill()
		return reg, err
	}
	var version struct {
		Version string `json:"version"`
	}
	_ = json.Unmarshal(raw, &version)
	if version.Version != w.Manifest.Version {
		w.kill()
		return reg, errors.New("plugin version differs from manifest")
	}
	return reg, nil
}
func (w *Worker) Handle(ctx context.Context, c Call) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	command, args, _ := ParseCommand(CommandText(c.Event))
	e := c.Event
	payload := map[string]any{"event": map[string]any{"botId": e.BotID, "eventType": e.Type, "eventId": e.EventID, "platformType": e.PlatformType, "data": e.Raw, "messageId": e.MessageID, "source": e.Source, "userId": e.UserID, "groupId": e.GroupID, "channelId": e.ChannelID, "guildId": e.GuildID, "message": e.Message, "botMentionIds": mentionIDs(e)}, "command": command, "args": args}
	_, err := w.request(ctx, "event", payload, &c)
	if err != nil {
		w.kill()
	}
	return err
}
func (w *Worker) request(ctx context.Context, typ string, payload any, c *Call) (json.RawMessage, error) {
	if w.cmd == nil {
		return nil, errors.New("worker not running")
	}
	id := fmt.Sprint(w.seq.Add(1))
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	if len(raw) > 1<<19 {
		return nil, errors.New("IPC payload too large")
	}
	if err = w.sendContext(ctx, Frame{V: 1, ID: id, Type: typ, Payload: raw}); err != nil {
		return nil, err
	}
	for calls := 0; ; {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case f, ok := <-w.inbox:
			if !ok {
				return nil, errors.New("worker exited or sent invalid protocol")
			}
			if f.Type == "result" {
				if f.ID != id {
					return nil, errors.New("IPC response ID mismatch")
				}
				if f.Error != "" {
					return nil, errors.New(f.Error)
				}
				return f.Payload, nil
			}
			calls++
			if calls > 128 || f.Parent != id || c == nil {
				return nil, errors.New("invalid IPC capability request")
			}
			value, capErr := w.capability(ctx, c, f)
			data, marshalErr := json.Marshal(value)
			if marshalErr != nil {
				return nil, marshalErr
			}
			response := Frame{V: 1, ID: f.ID, Parent: id, Type: "capability_result", Payload: data}
			if capErr != nil {
				response.Error = capErr.Error()
			}
			if err = w.sendContext(ctx, response); err != nil {
				return nil, err
			}
		}
	}
}
func (w *Worker) capability(ctx context.Context, c *Call, f Frame) (any, error) {
	switch f.Type {
	case "reply":
		var r events.Reply
		if json.Unmarshal(f.Payload, &r) != nil {
			return nil, errors.New("invalid reply")
		}
		if !w.Manifest.Allows("message.send") || c.Event.Reply == nil {
			return nil, errors.New("message.send denied")
		}
		switch r.Kind {
		case "text", "image", "rich_media", "markdown", "keyboard", "video":
		default:
			return nil, errors.New("unsupported reply kind")
		}
		if len(r.Text)+len(r.URL)+len(r.Keyboard) > 65536 {
			return nil, errors.New("reply too large")
		}
		return nil, c.Event.Reply(ctx, r)
	case "state.get", "state.set":
		var p struct {
			Key   string          `json:"key"`
			Value json.RawMessage `json:"value"`
		}
		if json.Unmarshal(f.Payload, &p) != nil || len(p.Key) > 128 {
			return nil, errors.New("invalid state key")
		}
		permission := "state.read"
		if f.Type == "state.set" {
			permission = "state.write"
		}
		if !w.Manifest.Allows(permission) || c.State == nil {
			return nil, errors.New("state access denied")
		}
		if f.Type == "state.set" {
			return nil, c.State.Set(ctx, p.Key, p.Value)
		}
		v, err := c.State.Get(ctx, p.Key)
		if len(v) == 0 {
			v = json.RawMessage("null")
		}
		return v, err
	case "log":
		if !w.Manifest.Allows("log.write") {
			return nil, errors.New("log.write denied")
		}
		var p struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(f.Payload, &p) != nil || len(p.Message) > 4096 {
			return nil, errors.New("invalid log")
		}
		if c.Log != nil {
			c.Log(p.Message)
		}
		return nil, nil
	}
	return nil, errors.New("unknown IPC capability")
}
func (w *Worker) send(f Frame) error {
	data, err := json.Marshal(f)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = w.input.Write(data)
	return err
}
func (w *Worker) kill() {
	if w.cmd == nil {
		return
	}
	_ = w.cmd.Process.Kill()
	_ = w.input.Close()
	close(w.done)
	select {
	case <-w.wait:
	case <-time.After(2 * time.Second):
	}
	os.RemoveAll(w.dir)
	w.cmd = nil
}
func (w *Worker) Stop(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.cmd == nil {
		return nil
	}
	_, err := w.request(ctx, "stop", nil, nil)
	w.kill()
	return err
}

// Plugin stderr must not block or consume unbounded host memory.
type discardBounded struct {
	mu        sync.Mutex
	log       func(string)
	remaining int
	buffer    string
}

func (d *discardBounded) Write(p []byte) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	size := len(p)
	if d.remaining <= 0 {
		return size, nil
	}
	n := len(p)
	if n > d.remaining {
		n = d.remaining
	}
	d.remaining -= n
	d.buffer += string(p[:n])
	for len(d.buffer) > 0 {
		index := strings.IndexByte(d.buffer, '\n')
		if index < 0 && len(d.buffer) < 4096 {
			break
		}
		if index < 0 || index > 4096 {
			index = 4096
		}
		if d.log != nil {
			d.log(d.buffer[:index])
		}
		if index < len(d.buffer) && d.buffer[index] == '\n' {
			index++
		}
		d.buffer = d.buffer[index:]
	}
	return size, nil
}

func (w *Worker) sendContext(ctx context.Context, f Frame) error {
	done := make(chan error, 1)
	go func() { done <- w.send(f) }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		_ = w.input.Close()
		<-done
		return ctx.Err()
	}
}

func fileModuleURL(file string) string {
	p := filepath.ToSlash(file)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return (&url.URL{Scheme: "file", Path: p}).String()
}

func workerEnvironment() []string {
	out := []string{"NODE_NO_WARNINGS=1"}
	for _, key := range []string{"PATH", "SystemRoot", "WINDIR", "TEMP", "TMP", "TZ"} {
		if value, ok := os.LookupEnv(key); ok {
			out = append(out, key+"="+value)
		}
	}
	return out
}
