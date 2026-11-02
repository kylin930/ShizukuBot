package api

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gorilla/mux"
	"io"
	"net/http"
	"shizukubot/plugins"
	"time"
)

type PluginInfo struct {
	plugins.Manifest
	Enabled         bool             `json:"enabled"`
	Instances       []plugins.Status `json:"instances"`
	Config          map[string]any   `json:"config,omitempty"`
	Compiled        bool             `json:"compiled"`
	BotID           string           `json:"bot_id,omitempty"`
	BotExists       bool             `json:"bot_exists"`
	BotRunning      bool             `json:"bot_running"`
	Allowed         bool             `json:"allowed"`
	Running         bool             `json:"running"`
	State           string           `json:"state"`
	LastError       string           `json:"last_error,omitempty"`
	ConfigError     string           `json:"config_error,omitempty"`
	RestartRequired bool             `json:"restart_required"`
}

func (r *Router) pluginAllowed(id, botID string) bool {
	bot, ok := r.botManager.GetBot(botID)
	if !ok {
		return false
	}
	ids := bot.ConfigSnapshot().Plugins
	if len(ids) == 0 {
		return true
	}
	for _, selected := range ids {
		if selected == id {
			return true
		}
	}
	return false
}
func (r *Router) pluginInfo(p plugins.Package, botID string) PluginInfo {
	out := PluginInfo{Manifest: p.Manifest, Compiled: true, BotID: botID, State: "unloaded", Instances: []plugins.Status{}}
	for _, status := range r.plugins.Statuses() {
		if status.PluginID == p.Manifest.ID {
			out.Instances = append(out.Instances, status)
			if botID == "" {
				out.Enabled = out.Enabled || status.Enabled
			}
			if status.BotID == botID {
				out.Running = status.Enabled
				out.State = status.State
				out.LastError = status.LastError
			}
		}
	}
	if botID != "" {
		settings := r.plugins.Config(p.Manifest.ID, botID)
		out.Enabled = settings.Enabled
		out.Config = settings.Config
		if effective, err := p.Manifest.ConfigSchema.Validate(out.Config); err == nil {
			out.Config = effective.(map[string]any)
		} else {
			out.ConfigError = err.Error()
		}
		if bot, ok := r.botManager.GetBot(botID); ok {
			out.BotExists = true
			out.BotRunning = bot.IsRunning()
			out.Allowed = r.pluginAllowed(p.Manifest.ID, botID)
		}
		out.RestartRequired = r.plugins.NeedsRestart(p.Manifest.ID, botID)
	}
	return out
}
func (r *Router) listPlugins(w http.ResponseWriter, req *http.Request, botID string, virtual bool) {
	if botID != "" && !virtual {
		if _, ok := r.botManager.GetBot(botID); !ok {
			NotFoundResponse(w, "bot not found")
			return
		}
	}
	list, err := r.plugins.Packages()
	if err != nil {
		InternalErrorResponse(w, "plugin catalog unavailable")
		return
	}
	out := []PluginInfo{}
	for _, p := range list {
		out = append(out, r.pluginInfo(p, botID))
	}
	SuccessResponse(w, out)
}
func (r *Router) GetPlugins(w http.ResponseWriter, req *http.Request) {
	r.listPlugins(w, req, req.URL.Query().Get("bot_id"), false)
}
func (r *Router) GetPlugin(w http.ResponseWriter, req *http.Request) {
	botID := req.URL.Query().Get("bot_id")
	if botID != "" {
		if _, ok := r.botManager.GetBot(botID); !ok {
			NotFoundResponse(w, "bot not found")
			return
		}
	}
	list, err := r.plugins.Packages()
	if err != nil {
		InternalErrorResponse(w, "plugin catalog unavailable")
		return
	}
	for _, p := range list {
		if p.Manifest.ID == mux.Vars(req)["id"] {
			SuccessResponse(w, r.pluginInfo(p, botID))
			return
		}
	}
	NotFoundResponse(w, "plugin not found")
}

// pluginOperation serializes changes for one plugin/Bot, never ordinary queries.
func (r *Router) pluginOperation(ctx context.Context, id, bot string) (func(), error) {
	value, _ := r.pluginOps.LoadOrStore(id+"\x00"+bot, make(chan struct{}, 1))
	gate := value.(chan struct{})
	select {
	case gate <- struct{}{}:
		return func() { <-gate }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
func (r *Router) requirePluginBot(w http.ResponseWriter, bot string) bool {
	if bot == "" {
		BadRequestResponse(w, "bot_id required")
		return false
	}
	if _, ok := r.botManager.GetBot(bot); !ok {
		NotFoundResponse(w, "bot not found")
		return false
	}
	return true
}
func (r *Router) CreatePlugin(w http.ResponseWriter, req *http.Request) {
	http.Error(w, "Plugins are compiled Go packages; register, rebuild and redeploy to add code.", http.StatusGone)
}
func (r *Router) RestorePluginData(w http.ResponseWriter, req *http.Request) {
	data, err := io.ReadAll(io.LimitReader(req.Body, plugins.MaxBackupBytes+1))
	var limitErr *http.MaxBytesError
	if errors.As(err, &limitErr) || len(data) > plugins.MaxBackupBytes {
		http.Error(w, "backup exceeds 4 MiB", http.StatusRequestEntityTooLarge)
		return
	}
	if err != nil {
		BadRequestResponse(w, "invalid data backup")
		return
	}
	var metadata struct {
		Bots map[string]json.RawMessage `json:"bots"`
	}
	if json.Unmarshal(data, &metadata) != nil {
		BadRequestResponse(w, "invalid data backup")
		return
	}
	for bot := range metadata.Bots {
		if !r.requirePluginBot(w, bot) {
			return
		}
	}
	if err = r.plugins.RestoreBackup(data); err != nil {
		BadRequestResponse(w, err.Error())
		return
	}
	SuccessResponse(w, map[string]bool{"enabled": false})
}
func (r *Router) UpdatePlugin(w http.ResponseWriter, req *http.Request) {
	id := mux.Vars(req)["id"]
	var body struct {
		BotID  string         `json:"bot_id"`
		Config map[string]any `json:"config"`
		Apply  bool           `json:"apply"`
	}
	if json.NewDecoder(req.Body).Decode(&body) != nil || body.Config == nil {
		BadRequestResponse(w, "bot_id and configuration object required")
		return
	}
	if !r.requirePluginBot(w, body.BotID) {
		return
	}
	ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
	defer cancel()
	release, err := r.pluginOperation(ctx, id, body.BotID)
	if err != nil {
		BadRequestResponse(w, err.Error())
		return
	}
	defer release()
	if err = r.plugins.Configure(id, body.BotID, body.Config); err != nil {
		BadRequestResponse(w, err.Error())
		return
	}
	result := map[string]any{"saved": true, "applied": false, "restart_required": r.plugins.NeedsRestart(id, body.BotID)}
	if body.Apply && r.plugins.NeedsRestart(id, body.BotID) {
		if !r.pluginAllowed(id, body.BotID) {
			result["restart_error"] = "plugin excluded by bot selection policy"
		} else if err = r.plugins.Restart(ctx, id, body.BotID); err != nil {
			result["restart_error"] = err.Error()
		} else {
			result["applied"] = true
		}
		result["restart_required"] = r.plugins.NeedsRestart(id, body.BotID)
	}
	SuccessResponse(w, result)
}
func (r *Router) TogglePlugin(w http.ResponseWriter, req *http.Request) {
	id := mux.Vars(req)["id"]
	var body struct {
		BotID   string `json:"bot_id"`
		Enabled bool   `json:"enabled"`
	}
	if json.NewDecoder(req.Body).Decode(&body) != nil {
		BadRequestResponse(w, "invalid toggle request")
		return
	}
	if !r.requirePluginBot(w, body.BotID) {
		return
	}
	if body.Enabled && !r.pluginAllowed(id, body.BotID) {
		BadRequestResponse(w, "plugin excluded by bot selection policy")
		return
	}
	ctx, cancel := context.WithTimeout(req.Context(), 4*time.Second)
	defer cancel()
	release, err := r.pluginOperation(ctx, id, body.BotID)
	if err != nil {
		BadRequestResponse(w, err.Error())
		return
	}
	defer release()
	if body.Enabled {
		err = r.plugins.Enable(ctx, id, body.BotID)
	} else {
		err = r.plugins.Disable(ctx, id, body.BotID)
	}
	if err != nil {
		BadRequestResponse(w, err.Error())
		return
	}
	SuccessResponse(w, r.plugins.Statuses())
}
func (r *Router) RestartPlugin(w http.ResponseWriter, req *http.Request) {
	id := mux.Vars(req)["id"]
	var body struct {
		BotID string `json:"bot_id"`
	}
	if json.NewDecoder(req.Body).Decode(&body) != nil {
		BadRequestResponse(w, "invalid restart request")
		return
	}
	if !r.requirePluginBot(w, body.BotID) {
		return
	}
	if !r.pluginAllowed(id, body.BotID) {
		BadRequestResponse(w, "plugin excluded by bot selection policy")
		return
	}
	ctx, cancel := context.WithTimeout(req.Context(), 4*time.Second)
	defer cancel()
	release, err := r.pluginOperation(ctx, id, body.BotID)
	if err != nil {
		BadRequestResponse(w, err.Error())
		return
	}
	defer release()
	if err = r.plugins.Restart(ctx, id, body.BotID); err != nil {
		BadRequestResponse(w, err.Error())
		return
	}
	SuccessResponse(w, nil)
}
func (r *Router) DeletePlugin(w http.ResponseWriter, req *http.Request) {
	http.Error(w, "Compiled plugins can be disabled; removing code requires rebuilding and redeploying.", http.StatusGone)
}
func (r *Router) ExportPlugin(w http.ResponseWriter, req *http.Request) {
	id := mux.Vars(req)["id"]
	data, err := r.plugins.ExportBackup(id)
	if err != nil {
		BadRequestResponse(w, "data backup failed")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="`+id+`-data.json"`)
	w.Write(data)
}
func (r *Router) Close(ctx context.Context) error { return r.plugins.Close(ctx) }
