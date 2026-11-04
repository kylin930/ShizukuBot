package core

import (
	"context"
	"shizukubot/config"
	"shizukubot/events"
	"strings"
	"testing"
)

func TestBuiltinControlsAndHelpListing(t *testing.T) {
	previous := currentTemplates()
	t.Cleanup(func() { SetCommandTemplates(&previous) })
	enabled, disabled := true, false
	bot := &Bot{ID: "test", Name: "test", CommandPrefix: "!", Stats: NewStats(), Commands: func([]string, string) []string { return []string{"!ping", "#shizuku"} }}
	run := func(message string, want bool) string {
		t.Helper()
		reply := ""
		event := events.Event{Message: message, Reply: func(_ context.Context, r events.Reply) error { reply = r.Text; return nil }}
		if got := bot.MatchesBuiltin(event); got != want {
			t.Fatalf("reserved %q: %v", message, got)
		}
		if got := bot.HandleBuiltinCommand(context.Background(), event); got != want {
			t.Fatalf("handled %q: %v", message, got)
		}
		if want && reply == "" {
			t.Fatal("no reply")
		}
		return reply
	}
	ConfigureBuiltinCommands(config.BuiltinCommandsConfig{})
	help := run("!help", true)
	for _, part := range []string{"!help", "!status", "#shizuku", "!ping"} {
		if !strings.Contains(help, part) {
			t.Fatal(help)
		}
	}
	ConfigureBuiltinCommands(config.BuiltinCommandsConfig{HelpEnabled: &disabled, StatusEnabled: &disabled, ShowShizukuInHelp: &disabled})
	run("!help", false)
	run("!status", false)
	run("#shizuku", true)
	ConfigureBuiltinCommands(config.BuiltinCommandsConfig{HelpEnabled: &enabled, StatusEnabled: &disabled, ShowShizukuInHelp: &disabled})
	help = run("!help", true)
	if strings.Contains(help, "!status") || strings.Contains(help, "#shizuku") || !strings.Contains(help, "!ping") {
		t.Fatal(help)
	}
	run("#shizuku more", true)
	run("!statusmore", false)
	// Caller-owned snapshots cannot mutate the live command settings.
	settings := &CommandTemplates{DisableHelp: true}
	SetCommandTemplates(settings)
	settings.DisableHelp = false
	run("!help", false)
}
