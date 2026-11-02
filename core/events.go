package core

import (
	"context"
	"errors"
	"fmt"
	"github.com/tencent-connect/botgo/dto"
	"shizukubot/transport/qq"
)

// Start creates one lifecycle per bot; StartAll skips configured disabled bots.
func (b *Bot) Start() error {
	b.lifecycle.Lock()
	defer b.lifecycle.Unlock()
	b.mu.Lock()
	if b.Running {
		b.mu.Unlock()
		return fmt.Errorf("bot %s is already running", b.ID)
	}
	ctx, cancel := context.WithCancel(context.Background())
	b.ctx, b.cancel = ctx, cancel
	source, api, mode := b.TokenSource, b.OpenAPI, b.Mode
	b.mu.Unlock()
	// The SDK token source refreshes lazily on Token calls. Avoid its global
	// background refresh helper, which logs credentials and can panic on failures.
	if _, err := source.Token(); err != nil {
		cancel()
		b.RecordTransportError("authentication_failed")
		return errors.New("bot authentication failed")
	}
	b.mu.Lock()
	b.Running = true
	b.Enabled = true
	b.mu.Unlock()
	if mode == "websocket" {
		g := &qq.Gateway{Tokens: source, Intents: dto.IntentGroupMessages | dto.IntentGuildAtMessage | dto.IntentDirectMessages | dto.IntentInteraction,
			Discover: func(ctx context.Context) (*dto.WebsocketAP, error) { return api.WS(ctx, nil, "") },
			Dispatch: b.HandlePayload, OnError: func(err error) { b.RecordTransportError("gateway_connection_failed") }}
		b.workers.Add(1)
		go func() {
			defer b.workers.Done()
			g.Run(ctx)
			b.mu.Lock()
			if b.ctx == ctx {
				b.Running = false
			}
			b.mu.Unlock()
		}()
	}
	b.logInfo("bot started in %s mode", mode)
	return nil
}

// Stop cancels event work, closes connections and joins reconnect workers.
func (b *Bot) Stop() {
	b.lifecycle.Lock()
	defer b.lifecycle.Unlock()
	b.mu.Lock()
	if b.cancel != nil {
		b.cancel()
	}
	b.Running = false
	b.Enabled = false
	b.mu.Unlock()
	b.workers.Wait()
	b.logInfo("bot stopped")
}

// HandlePayload is the per-bot entrance shared by both transports.
func (b *Bot) HandlePayload(ctx context.Context, raw []byte) error {
	_, err := b.HandlePayloadAccepted(ctx, raw)
	return err
}

// HandlePayloadAccepted separates event admission from downstream business errors.
func (b *Bot) HandlePayloadAccepted(ctx context.Context, raw []byte) (bool, error) {
	b.mu.Lock()
	running := b.Running
	life := b.ctx
	dispatcher := b.Dispatcher
	ids := append([]string(nil), b.PluginIDs...)
	if running {
		b.workers.Add(1)
	}
	b.mu.Unlock()
	if running {
		defer b.workers.Done()
	}
	if !running || life == nil {
		return false, errors.New("bot is stopped")
	}
	if dispatcher == nil {
		return false, errors.New("event dispatcher unavailable")
	}
	call, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(life, cancel)
	defer stop()
	defer cancel()
	e, err := qq.Normalize(b.ID, raw)
	if err != nil {
		return false, err
	}
	if err := call.Err(); err != nil {
		return false, err
	}
	e.PluginIDs = ids
	e.Reply = b.replyContext(call, e)
	b.recordEvent(e)
	return true, dispatcher.Dispatch(call, e)
}
