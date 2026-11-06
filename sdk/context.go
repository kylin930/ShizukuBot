package sdk

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"time"
)

// Call is the capability-limited context visible to trusted extensions.
type Call struct {
	Event  Event
	Args   []string
	Config map[string]any
	State  State
	Log    func(string)
}

// State is isolated per plugin and bot. Values must be JSON serializable.
type State interface {
	// Get returns JSON, or nil when the key does not exist.
	Get(context.Context, string) (json.RawMessage, error)
	// Set persists valid JSON subject to the host storage limits.
	Set(context.Context, string, json.RawMessage) error
}

// Reply sends through the host-provided capability (capture-only in the sandbox).
func (c Call) Reply(ctx context.Context, text string) error {
	return c.Send(ctx, Reply{Kind: "text", Text: text})
}

// Send supports text and rich-media replies without exposing a QQ client.
func (c Call) Send(ctx context.Context, reply Reply) error {
	if c.Event.Reply == nil {
		return errors.New("reply capability unavailable")
	}
	return c.Event.Reply(ctx, reply)
}

// ReplyMarkdown sends Markdown through the same permission/source checks as Send.
func (c Call) ReplyMarkdown(ctx context.Context, text string) error {
	return c.Send(ctx, Reply{Kind: "markdown", Text: text})
}

// ReplyImage sends an image supported by the current source's host adapter.
// Unsupported sources and capability failures are returned to the caller.
func (c Call) ReplyImage(ctx context.Context, url string) error {
	return c.Send(ctx, Reply{Kind: "image", URL: url})
}

// ReplyAt replies with text that mentions userID. QQ groups deliver this as a
// Markdown message; guilds keep the text message type. C2C mentions fail.
func (c Call) ReplyAt(ctx context.Context, text, userID string) error {
	return c.Send(ctx, Reply{Kind: "text", Text: text, Mention: userID})
}

// ReplyImageBytes sends in-memory image bytes without writing a temporary file.
// QQ direct binary upload accepts at most 5 MiB.
func (c Call) ReplyImageBytes(ctx context.Context, data []byte) error {
	if len(data) == 0 {
		return errors.New("empty image data")
	}
	if len(data) > 5<<20 {
		return errors.New("image data exceeds the 5 MiB inline upload limit")
	}
	mime := http.DetectContentType(data)
	if len(mime) < 6 || mime[:6] != "image/" {
		return errors.New("unsupported image data")
	}
	return c.Send(ctx, Reply{Kind: "image", URL: "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)})
}

// ReplyImageFile reads a local image and sends it through the normal media path.
func (c Call) ReplyImageFile(ctx context.Context, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return c.ReplyImageBytes(ctx, data)
}

// Recall removes the message that triggered this event.
func (c Call) Recall(ctx context.Context) error {
	return c.RecallMessage(ctx, c.Event.MessageID)
}

// RecallMessage removes one message in the current conversation. Group
// administrators may also recall messages sent by members.
func (c Call) RecallMessage(ctx context.Context, messageID string) error {
	if messageID == "" {
		return errors.New("message ID required")
	}
	return c.act(ctx, Action{Kind: ActionRecall, MessageID: messageID})
}

// Mute restricts the user who triggered this event.
func (c Call) Mute(ctx context.Context, duration time.Duration) error {
	return c.MuteUser(ctx, c.Event.UserID, duration)
}

// MuteUser restricts userID for duration. Group durations are capped by QQ at 30 days.
func (c Call) MuteUser(ctx context.Context, userID string, duration time.Duration) error {
	if userID == "" {
		return errors.New("user ID required")
	}
	seconds := int64(duration / time.Second)
	if seconds <= 0 {
		return errors.New("mute duration must be at least one second")
	}
	return c.act(ctx, Action{Kind: ActionMute, UserID: userID, Seconds: seconds})
}

// Unmute clears the restriction on the user who triggered this event.
func (c Call) Unmute(ctx context.Context) error {
	return c.UnmuteUser(ctx, c.Event.UserID)
}

// UnmuteUser clears a member restriction.
func (c Call) UnmuteUser(ctx context.Context, userID string) error {
	if userID == "" {
		return errors.New("user ID required")
	}
	return c.act(ctx, Action{Kind: ActionUnmute, UserID: userID})
}

// Kick removes the user who triggered this event from the current group or guild.
func (c Call) Kick(ctx context.Context, blacklist bool) error {
	return c.KickUser(ctx, c.Event.UserID, blacklist)
}

// KickUser removes one member. Group removal is whitelist-gated by QQ.
func (c Call) KickUser(ctx context.Context, userID string, blacklist bool) error {
	if userID == "" {
		return errors.New("user ID required")
	}
	return c.act(ctx, Action{Kind: ActionKick, UserID: userID, Blacklist: blacklist})
}

func (c Call) act(ctx context.Context, action Action) error {
	if c.Event.Act == nil {
		return errors.New("management capability unavailable")
	}
	return c.Event.Act(ctx, action)
}
