package sdk

import (
	"context"
	"encoding/json"
	"errors"
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
