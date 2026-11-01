package plugins

import (
	"shizukubot/events"
	"shizukubot/sdk"
)

// ParseCommand is retained for source compatibility.
//
// Deprecated: use sdk.ParseCommand.
func ParseCommand(message string) (string, []string, bool) { return sdk.ParseCommand(message) }

// CommandText is retained for source compatibility.
//
// Deprecated: use sdk.CommandText.
func CommandText(e events.Event) string { return sdk.CommandText(e) }
