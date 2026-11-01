package api

import (
	"context"
	"encoding/json"
	"github.com/gorilla/mux"
	"net/http"
	"shizukubot/events"
	"shizukubot/plugin"
)

type SandboxMessageRequest struct {
	Type     string `json:"type"`
	Content  string `json:"content"`
	PluginID string `json:"plugin_id"`
}
type SandboxMessageResponse struct {
	Replies    []string                 `json:"replies"`
	Results    []events.Reply           `json:"results"`
	Errors     []string                 `json:"errors"`
	Executions []plugin.ExecutionResult `json:"executions"`
}

// SandboxSend has capture-only capabilities. It never obtains a real Bot sender.
func (r *Router) SandboxSend(w http.ResponseWriter, req *http.Request) {
	var body SandboxMessageRequest
	if json.NewDecoder(req.Body).Decode(&body) != nil || body.Content == "" || len(body.Content) > 65536 {
		BadRequestResponse(w, "invalid sandbox message")
		return
	}
	if body.Type == "" {
		body.Type = "c2c"
	}
	if body.Type != "c2c" && body.Type != "group" {
		BadRequestResponse(w, "invalid source type")
		return
	}
	botID := mux.Vars(req)["id"]
	if botID == "" {
		BadRequestResponse(w, "bot id required")
		return
	}
	out := SandboxMessageResponse{Replies: []string{}, Results: []events.Reply{}, Errors: []string{}}
	e := events.Event{BotID: botID, Type: "c2c_message", Source: events.C2C, Message: body.Content, UserID: "debug-user"}
	if bot, ok := r.botManager.GetBot(botID); ok {
		e.PluginIDs = bot.ConfigSnapshot().Plugins
	}
	if body.Type == "group" {
		e.Type = "group_at_message"
		e.Source = events.Group
		e.GroupID = "debug-group"
	}
	e.Reply = func(_ context.Context, m events.Reply) error {
		out.Results = append(out.Results, m)
		s := m.Text
		switch m.Kind {
		case "image", "video":
			s = "[" + m.Kind + "] " + m.URL
		case "rich_media":
			s += "\n[image] " + m.URL
		}
		out.Replies = append(out.Replies, s)
		return nil
	}
	adapter := &plugin.LegacyAdapter{Engine: r.engine, LogDebug: r.logManager.Debug}
	// Use a separate bus: production subscribers and reply capabilities are never exposed.
	bus := events.NewBus()
	bus.Subscribe(events.Filter{BotID: botID}, func(ctx context.Context, e events.Event) error {
		var err error
		out.Executions, err = adapter.Run(ctx, e, body.PluginID)
		return err
	})
	if err := bus.Publish(req.Context(), e); err != nil {
		out.Errors = append(out.Errors, "legacy execution failed")
		r.logManager.Error(botID, "sandbox legacy execution failed")
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}
func (r *Router) SandboxPlugins(w http.ResponseWriter, req *http.Request) {
	plugins := r.pluginStore.List()
	if plugins == nil {
		plugins = []*plugin.Plugin{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(plugins)
}
