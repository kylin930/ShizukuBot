package qq

import (
	"context"
	"encoding/json"
	"github.com/gorilla/websocket"
	"github.com/kylin930/botgo-plus/dto"
	"golang.org/x/oauth2"
	"net/http"
	"net/http/httptest"
	"shizukubot/events"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestGatewayReconnectResumeAndShutdown(t *testing.T) {
	var connections atomic.Int32
	resumed := make(chan struct{}, 1)
	got := make(chan string, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		n := connections.Add(1)
		c.WriteJSON(map[string]any{"op": 10, "d": map[string]any{"heartbeat_interval": 1000}})
		var p struct {
			Op   int            `json:"op"`
			Data map[string]any `json:"d"`
		}
		if c.ReadJSON(&p) != nil {
			return
		}
		if n == 1 {
			if p.Op != 2 {
				t.Error("initial connection did not identify")
			}
			c.WriteJSON(map[string]any{"op": 0, "t": "READY", "s": 1, "d": map[string]any{"session_id": "session"}})
		} else {
			if p.Op != 6 || p.Data["session_id"] != "session" || p.Data["token"] != "fake" || p.Data["seq"] != float64(2) {
				t.Error("resume state lost")
			}
			resumed <- struct{}{}
		}
		c.WriteJSON(map[string]any{"op": 0, "t": "C2C_MESSAGE_CREATE", "s": 2, "d": map[string]any{"id": "same-id", "content": "same"}})
		if n == 1 {
			c.WriteJSON(map[string]any{"op": 7})
			return
		}
		c.WriteJSON(map[string]any{"op": 0, "t": "C2C_MESSAGE_CREATE", "s": 3, "d": map[string]any{"id": "next-id", "content": "same"}})
		for {
			var beat json.RawMessage
			if c.ReadJSON(&beat) != nil {
				return
			}
			c.WriteJSON(map[string]any{"op": 11})
		}
	}))
	defer server.Close()
	bus := events.NewBus()
	d := events.NewDispatcher(bus)
	bus.Subscribe(events.Filter{BotID: "bot"}, func(_ context.Context, e events.Event) error { got <- e.MessageID; return nil })
	g := Gateway{Discover: func(context.Context) (*dto.WebsocketAP, error) {
		return &dto.WebsocketAP{URL: "ws" + strings.TrimPrefix(server.URL, "http"), Shards: 1, SessionStartLimit: dto.SessionStartLimit{Remaining: 10, MaxConcurrency: 1}}, nil
	}, Tokens: oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "fake", TokenType: "QQBot"}), Retry: time.Millisecond, Dispatch: func(ctx context.Context, raw []byte) error {
		e, err := Normalize("bot", raw)
		if err != nil {
			return err
		}
		return d.Dispatch(ctx, e)
	}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { defer close(done); g.Run(ctx) }()
	for _, want := range []string{"same-id", "next-id"} {
		select {
		case id := <-got:
			if id != want {
				t.Fatalf("got %s want %s", id, want)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("event not received")
		}
	}
	select {
	case <-resumed:
	case <-time.After(time.Second):
		t.Fatal("did not resume")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("transport workers leaked")
	}
	if connections.Load() != 2 {
		t.Fatalf("connections %d", connections.Load())
	}
	select {
	case extra := <-got:
		t.Fatal("duplicate", extra)
	default:
	}
}
func TestGatewayDiscoveryCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	g := Gateway{Discover: func(context.Context) (*dto.WebsocketAP, error) { called = true; return nil, nil }}
	g.Run(ctx)
	if called {
		t.Fatal("discovery after cancellation")
	}
}

func TestGatewayIdentifyHeartbeatAndServerHeartbeat(t *testing.T) {
	verified := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		conn.WriteJSON(map[string]any{"op": 10, "d": map[string]any{"heartbeat_interval": 100}})
		var identify struct {
			Op   int `json:"op"`
			Data struct {
				Token string   `json:"token"`
				Shard []uint32 `json:"shard"`
			} `json:"d"`
		}
		if err = conn.ReadJSON(&identify); err != nil {
			return
		}
		if identify.Op != 2 || identify.Data.Token != "QQBot fake" || len(identify.Data.Shard) != 2 || identify.Data.Shard[0] != 0 || identify.Data.Shard[1] != 1 {
			t.Error("invalid Identify token/shard")
		}
		conn.WriteJSON(map[string]any{"op": 0, "t": "READY", "s": 7, "d": map[string]any{"session_id": "session"}})
		conn.WriteJSON(map[string]any{"op": 1})
		for range 2 {
			var beat struct {
				Op   int    `json:"op"`
				Data uint32 `json:"d"`
			}
			if err = conn.ReadJSON(&beat); err != nil {
				return
			}
			if beat.Op != 1 || beat.Data != 7 {
				t.Error("heartbeat lost received sequence")
			}
			conn.WriteJSON(map[string]any{"op": 11})
		}
		verified <- struct{}{}
		// Read must be unblocked by cancellation and connection close.
		for {
			if _, _, err = conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer server.Close()
	g := Gateway{Tokens: oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "fake", TokenType: "QQBot"}), Discover: func(context.Context) (*dto.WebsocketAP, error) {
		return &dto.WebsocketAP{URL: "ws" + strings.TrimPrefix(server.URL, "http"), Shards: 1, SessionStartLimit: dto.SessionStartLimit{Remaining: 2}}, nil
	}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { defer close(done); g.Run(ctx) }()
	select {
	case <-verified:
	case <-time.After(3 * time.Second):
		t.Fatal("heartbeat exchange timed out")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("gateway did not join heartbeat worker")
	}
}
