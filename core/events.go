package core

import (
	"context"
	"errors"
	"fmt"
	"github.com/tencent-connect/botgo/dto"
	"shizukubot/events"
	"shizukubot/transport/qq"
)

// Start creates one lifecycle per bot. Disabled bots are deliberately left stopped.
func (b *Bot) Start() error {
	b.lifecycle.Lock()
	defer b.lifecycle.Unlock()
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.Running {
		return fmt.Errorf("bot %s is already running", b.ID)
	}
	b.ctx, b.cancel = context.WithCancel(context.Background())
	// The SDK token source refreshes lazily on Token calls. Avoid its global
	// background refresh helper, which logs credentials and can panic on failures.
	if _, err := b.TokenSource.Token(); err != nil {
		b.cancel()
		return errors.New("bot authentication failed")
	}
	b.Running = true
	b.Enabled = true
	if b.Mode == "websocket" {
		g := &qq.Gateway{Tokens: b.TokenSource, Intents: dto.IntentGroupMessages | dto.IntentGuildAtMessage | dto.IntentDirectMessages | dto.IntentInteraction,
			Discover: func(ctx context.Context) (*dto.WebsocketAP, error) { return b.OpenAPI.WS(ctx, nil, "") },
			Dispatch: b.HandlePayload, OnError: func(err error) { b.logError("%v", err) }}
		ctx := b.ctx
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
	b.logInfo("bot started in %s mode", b.Mode)
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
		return errors.New("bot is stopped")
	}
	if dispatcher == nil {
		return errors.New("event dispatcher unavailable")
	}
	call, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(life, cancel)
	defer stop()
	defer cancel()
	e, err := qq.Normalize(b.ID, raw)
	if err != nil {
		return err
	}
	e.PluginIDs = ids
	e.Reply = b.reply(e)
	return dispatcher.Dispatch(call, e)
}
func (b *Bot) reply(e events.Event) func(context.Context, events.Reply) error {
	return func(ctx context.Context, r events.Reply) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		switch e.Source {
		case events.Group:
			switch r.Kind {
			case "text":
				return b.sendGroupMessageContext(ctx, e.GroupID, r.Text)
			case "image":
				return b.sendGroupImageContext(ctx, e.GroupID, r.URL)
			case "rich_media":
				return b.sendGroupRichMediaContext(ctx, e.GroupID, r.Text, r.URL)
			case "markdown":
				return b.sendGroupMarkdownContext(ctx, e.GroupID, r.Text)
			case "keyboard":
				return b.sendGroupKeyboardContext(ctx, e.GroupID, r.Text, r.Keyboard)
			case "video":
				return b.sendGroupVideoContext(ctx, e.GroupID, r.URL)
			}
		case events.C2C:
			switch r.Kind {
			case "text":
				return b.sendC2CMessageContext(ctx, e.UserID, r.Text)
			case "image":
				return b.sendC2CImageContext(ctx, e.UserID, r.URL)
			case "rich_media":
				return b.sendC2CRichMediaContext(ctx, e.UserID, r.Text, r.URL)
			case "markdown":
				return b.sendC2CMarkdownContext(ctx, e.UserID, r.Text)
			case "keyboard":
				return b.sendC2CKeyboardContext(ctx, e.UserID, r.Text, r.Keyboard)
			case "video":
				return b.sendC2CVideoContext(ctx, e.UserID, r.URL)
			}
		case events.Guild, events.Direct:
			if r.Kind == "text" {
				m := &dto.MessageToCreate{Content: r.Text, MsgID: e.MessageID}
				var err error
				if e.Source == events.Guild {
					_, err = b.OpenAPI.PostMessage(ctx, e.ChannelID, m)
				} else {
					_, err = b.OpenAPI.PostDirectMessage(ctx, &dto.DirectMessage{GuildID: e.GuildID}, m)
				}
				return err
			}
		}
		return errors.New("reply kind unsupported for this source")
	}
}
