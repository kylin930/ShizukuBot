package api

import (
	"context"
	"shizukubot/config"
	"shizukubot/core"
	"shizukubot/events"
	"testing"
)

func TestBuiltinSwitchConfigRoundTripAndRouting(t *testing.T) {
	core.ConfigureBuiltinCommands(config.BuiltinCommandsConfig{})
	t.Cleanup(func() { core.ConfigureBuiltinCommands(config.BuiltinCommandsConfig{}) })
	r, c := setupRouter(t)
	addManagementBot(t, r, "a", nil)
	bot, _ := r.botManager.GetBot("a")
	cfg := apiData(t, request(r, c, "GET", "/api/config", ""))
	flags := cfg["builtin_commands"].(map[string]interface{})
	for _, key := range []string{"help_enabled", "status_enabled", "show_shizuku_in_help"} {
		if flags[key] != true {
			t.Fatal(key, flags)
		}
	}
	flags["help_enabled"] = false
	flags["status_enabled"] = false
	flags["show_shizuku_in_help"] = false
	apiData(t, request(r, c, "PUT", "/api/config", mustJSON(cfg)))
	for _, message := range []string{"/help", "/status"} {
		e := events.Event{Message: message}
		if bot.MatchesBuiltin(e) || bot.HandleBuiltinCommand(context.Background(), e) {
			t.Fatal("disabled command was reserved", message)
		}
	}
	if !bot.MatchesBuiltin(events.Event{Message: "#shizuku"}) {
		t.Fatal("mandatory command disabled")
	}
	stored, err := config.LoadConfig("config/config.json")
	if err != nil || config.BuiltinEnabled(stored.BuiltinCommands.HelpEnabled) || config.BuiltinEnabled(stored.BuiltinCommands.StatusEnabled) || config.BuiltinEnabled(stored.BuiltinCommands.ShowShizukuInHelp) {
		t.Fatal(stored, err)
	}
	// Older clients editing templates must preserve newer switch fields.
	legacy := `{"webui":{"port":4545},"builtin_commands":{"help":"{指令列表}","status":"status","shizuku":"framework"}}`
	apiData(t, request(r, c, "PUT", "/api/config", legacy))
	after := apiData(t, request(r, c, "GET", "/api/config", ""))["builtin_commands"].(map[string]interface{})
	if after["help_enabled"] != false || after["status_enabled"] != false || after["show_shizuku_in_help"] != false {
		t.Fatal(after)
	}
	// Enabled templates and command switches share the existing optimistic save guard.
	current := apiData(t, request(r, c, "GET", "/api/config", ""))
	current["builtin_commands"].(map[string]interface{})["help_enabled"] = true
	apiData(t, request(r, c, "PUT", "/api/config", mustJSON(current)))
	if !bot.MatchesBuiltin(events.Event{Message: "/help"}) || bot.MatchesBuiltin(events.Event{Message: "/status"}) {
		t.Fatal("live switches did not apply")
	}
	if out := envelope(t, request(r, c, "PUT", "/api/config", mustJSON(current))); out.Code != 409 {
		t.Fatal(out)
	}
	event := events.Event{Message: "/help", Reply: func(_ context.Context, reply events.Reply) error {
		if reply.Text != "/help" {
			t.Error(reply.Text)
		}
		return nil
	}}
	if !bot.HandleBuiltinCommand(context.Background(), event) {
		t.Fatal("help failed")
	}
	// A configured rule can handle an unreserved, disabled builtin command.
	rule := apiRule()
	rule.Trigger.Type = "exact"
	rule.Trigger.Value = "/status"
	rule.Action.Text = "rule status"
	if out := envelope(t, request(r, c, "PUT", "/api/bots/a/rules", rulesBody(t, 0, rule))); out.Code != 0 {
		t.Fatal(out)
	}
	result, err := r.rules.Execute(context.Background(), events.Event{BotID: "a", Source: events.C2C, Type: "c2c_message", Message: "/status", Reply: func(context.Context, events.Reply) error { return nil }})
	if err != nil || len(result.Replies) != 1 {
		t.Fatal(result, err)
	}
}
