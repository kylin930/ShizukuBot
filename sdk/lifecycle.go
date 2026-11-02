package sdk

import "context"

// Host lets Go extensions register callbacks during Load or Start.
type Host interface {
	// Context remains live until Disable/Unload; use it for background resources.
	Context() context.Context
	// BotID identifies this instance without exposing a bot client.
	BotID() string
	// Config returns an isolated snapshot when config.read is granted.
	Config() map[string]any
	// State provides permission-checked storage for this plugin and Bot.
	State() State
	// Log writes through the host when log.write is granted.
	Log(string)
	Registrar
}

// Extension is a statically registered, trusted Go plugin. Methods honor context.
type Extension interface {
	Load(context.Context, Host) error
	Start(context.Context) error
	HandleEvent(context.Context, Call) error
	Stop(context.Context) error
	Unload(context.Context) error
}

// Base supplies no-op lifecycle hooks for plugins that only register callbacks.
// Embed Base and override the hooks your plugin needs.
type Base struct{}

// Load performs no initialization. Override it to register callbacks.
func (Base) Load(context.Context, Host) error { return nil }

// Start performs no startup work.
func (Base) Start(context.Context) error { return nil }

// HandleEvent performs no additional event work after registered callbacks.
func (Base) HandleEvent(context.Context, Call) error { return nil }

// Stop performs no cleanup. Resource-owning plugins should override it.
func (Base) Stop(context.Context) error { return nil }

// Unload performs no final cleanup.
func (Base) Unload(context.Context) error { return nil }
