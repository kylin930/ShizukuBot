package qq

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/kylin930/botgo-plus/dto"
	"golang.org/x/oauth2"
)

// Gateway owns all shard connections for a single bot. Run blocks until shutdown.
type Gateway struct {
	Discover func(context.Context) (*dto.WebsocketAP, error)
	Tokens   oauth2.TokenSource
	Intents  dto.Intent
	Dispatch func(context.Context, []byte) error
	OnError  func(error)
	Retry    time.Duration
}

func pause(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
func (g *Gateway) report(err error) {
	if g.OnError != nil && err != nil {
		g.OnError(err)
	}
}
func (g *Gateway) Run(ctx context.Context) {
	retry := g.Retry
	if retry <= 0 {
		retry = 5 * time.Second
	}
	for ctx.Err() == nil {
		ap, err := g.Discover(ctx)
		if err != nil {
			g.report(errors.New("gateway discovery failed"))
			if !pause(ctx, retry) {
				return
			}
			continue
		}
		if ap == nil || ap.Shards == 0 || ap.Shards > 128 {
			g.report(errors.New("invalid gateway shard count"))
			return
		}
		// A single launch at each interval is conservative even with a larger concurrency allowance.
		interval := 5 * time.Second
		if ap.SessionStartLimit.MaxConcurrency > 0 {
			interval /= time.Duration(ap.SessionStartLimit.MaxConcurrency)
		}
		if ap.SessionStartLimit.Remaining < ap.Shards {
			g.report(errors.New("gateway session start limit exhausted"))
			return
		}
		var wg sync.WaitGroup
		for shard := uint32(0); shard < ap.Shards; shard++ {
			if shard > 0 && !pause(ctx, interval) {
				break
			}
			wg.Add(1)
			go func(id uint32) { defer wg.Done(); g.shard(ctx, ap.URL, id, ap.Shards, retry) }(shard)
		}
		wg.Wait()
		return
	}
}

type resumeState struct {
	ID  string
	Seq uint32
}

func (g *Gateway) shard(ctx context.Context, url string, id, count uint32, retry time.Duration) {
	state := resumeState{}
	for ctx.Err() == nil {
		err := g.connect(ctx, url, id, count, &state)
		if ctx.Err() != nil {
			return
		}
		var closeErr *websocket.CloseError
		if errors.As(err, &closeErr) {
			switch closeErr.Code {
			case 4004, 4010, 4011, 4012, 4013, 4014, 4914, 4915:
				g.report(errors.New("gateway authentication or sharding rejected"))
				return
			case 4006, 4007, 4009:
				state = resumeState{}
			}
		}
		g.report(errors.New("gateway disconnected; reconnecting"))
		if !pause(ctx, retry) {
			return
		}
	}
}
func (g *Gateway) connect(ctx context.Context, url string, id, count uint32, state *resumeState) error {
	conn, resp, err := websocket.DefaultDialer.DialContext(ctx, url, nil)
	if resp != nil && resp.Body != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return err
	}
	defer conn.Close()
	conn.SetReadLimit(2 << 20)
	conn.SetReadDeadline(time.Now().Add(15 * time.Second))
	cctx, cancel := context.WithCancel(ctx)
	defer cancel()
	// Cancellation closes reads and joins every transport worker before reconnect.
	var workers sync.WaitGroup
	workers.Add(1)
	go func() { defer workers.Done(); <-cctx.Done(); conn.Close() }()
	defer workers.Wait()
	defer cancel()
	var writeMu sync.Mutex
	write := func(v any) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		return conn.WriteJSON(v)
	}
	var seqMu sync.Mutex
	heartbeatStarted := false
	heartbeatInterval := 60 * time.Second
	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		var p struct {
			Op   int             `json:"op"`
			Type string          `json:"t"`
			Seq  uint32          `json:"s"`
			Data json.RawMessage `json:"d"`
		}
		if err = json.Unmarshal(raw, &p); err != nil {
			return errors.New("invalid gateway packet")
		}
		switch p.Op {
		case 10:
			if heartbeatStarted {
				return errors.New("repeated gateway hello")
			}
			heartbeatStarted = true
			var hello struct {
				Interval int `json:"heartbeat_interval"`
			}
			if json.Unmarshal(p.Data, &hello) != nil || hello.Interval < 1 {
				return errors.New("invalid heartbeat interval")
			}
			heartbeatInterval = time.Duration(hello.Interval) * time.Millisecond
			conn.SetReadDeadline(time.Now().Add(2 * heartbeatInterval))
			workers.Add(1)
			go func(interval time.Duration) {
				defer workers.Done()
				ticker := time.NewTicker(interval)
				defer ticker.Stop()
				for {
					select {
					case <-cctx.Done():
						return
					case <-ticker.C:
						seqMu.Lock()
						seq := state.Seq
						seqMu.Unlock()
						if write(map[string]any{"op": 1, "d": seq}) != nil {
							conn.Close()
							return
						}
					}
				}
			}(time.Duration(hello.Interval) * time.Millisecond)
			tk, err := g.Tokens.Token()
			if err != nil {
				return errors.New("gateway token unavailable")
			}
			if state.ID != "" {
				err = write(map[string]any{"op": 6, "d": map[string]any{"token": tk.AccessToken, "session_id": state.ID, "seq": state.Seq}})
			} else {
				err = write(map[string]any{"op": 2, "d": map[string]any{"token": fmt.Sprintf("%s %s", tk.TokenType, tk.AccessToken), "intents": g.Intents, "shard": []uint32{id, count}}})
			}
			if err != nil {
				return err
			}
		case 1:
			seqMu.Lock()
			seq := state.Seq
			seqMu.Unlock()
			if err := write(map[string]any{"op": 1, "d": seq}); err != nil {
				return err
			}
		case 11:
			// ACK proves the connection is alive. Extend the read deadline on inbound packets below.
		case 7:
			return errors.New("gateway requested reconnect")
		case 9:
			state.ID = ""
			seqMu.Lock()
			state.Seq = 0
			seqMu.Unlock()
			return errors.New("gateway invalid session")
		case 0:
			if p.Type == "READY" {
				var ready struct {
					ID string `json:"session_id"`
				}
				if json.Unmarshal(p.Data, &ready) != nil {
					return errors.New("invalid ready")
				}
				state.ID = ready.ID
			} else if p.Type != "RESUMED" && g.Dispatch != nil {
				g.report(g.Dispatch(cctx, raw))
			}
		}
		if p.Op == 0 && p.Seq > 0 {
			seqMu.Lock()
			state.Seq = p.Seq
			seqMu.Unlock()
		}
		// A bounded idle deadline also catches missing heartbeat ACKs/traffic.

		if p.Op == 11 {
			conn.SetReadDeadline(time.Now().Add(2 * heartbeatInterval))
		}
	}
}
