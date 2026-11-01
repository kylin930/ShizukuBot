package api

import (
	"context"
	"encoding/json"
	"github.com/gorilla/mux"
	"io"
	"net/http"
	"shizukubot/plugins"
	"time"
)

type PluginInfo struct {
	plugins.Manifest
	Enabled   bool             `json:"enabled"`
	Instances []plugins.Status `json:"instances"`
	Config    map[string]any   `json:"config,omitempty"`
}

func (r *Router) pluginInfo(p plugins.Package, bot string) PluginInfo {
	out := PluginInfo{Manifest: p.Manifest, Instances: []plugins.Status{}}
	for _, s := range r.plugins.Statuses() {
		if s.PluginID == p.Manifest.ID {
			out.Instances = append(out.Instances, s)
			out.Enabled = out.Enabled || s.Enabled
		}
	}
	if bot != "" {
		settings := r.plugins.Config(p.Manifest.ID, bot)
		out.Config = settings.Config
		out.Enabled = settings.Enabled
	}
	return out
}
func (r *Router) GetPlugins(w http.ResponseWriter, req *http.Request) {
	list, err := r.plugins.Packages()
	if err != nil {
		InternalErrorResponse(w, "plugin store unavailable")
		return
	}
	out := []PluginInfo{}
	for _, p := range list {
		out = append(out, r.pluginInfo(p, req.URL.Query().Get("bot_id")))
	}
	SuccessResponse(w, out)
}
func (r *Router) GetPlugin(w http.ResponseWriter, req *http.Request) {
	id := mux.Vars(req)["id"]
	list, err := r.plugins.Packages()
	if err != nil {
		InternalErrorResponse(w, "plugin store unavailable")
		return
	}
	for _, p := range list {
		if p.Manifest.ID == id {
			SuccessResponse(w, r.pluginInfo(p, req.URL.Query().Get("bot_id")))
			return
		}
	}
	NotFoundResponse(w, "plugin not found")
}

// CreatePlugin explicitly retires runtime source uploads.
func (r *Router) CreatePlugin(w http.ResponseWriter, req *http.Request) {
	http.Error(w, "Plugins are compiled Go packages; register, rebuild and redeploy to add code.", http.StatusGone)
}
func (r *Router) RestorePluginData(w http.ResponseWriter, req *http.Request) {
	data, err := io.ReadAll(io.LimitReader(req.Body, (4<<20)+1))
	if err != nil || len(data) > 4<<20 {
		BadRequestResponse(w, "invalid data backup")
		return
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
	}
	if json.NewDecoder(req.Body).Decode(&body) != nil || body.BotID == "" {
		BadRequestResponse(w, "bot_id required")
		return
	}
	if body.Config != nil {
		if err := r.plugins.Configure(id, body.BotID, body.Config); err != nil {
			BadRequestResponse(w, err.Error())
			return
		}
	}
	SuccessResponse(w, map[string]any{"restart_required": true})
}
func (r *Router) TogglePlugin(w http.ResponseWriter, req *http.Request) {
	id := mux.Vars(req)["id"]
	var body struct {
		BotID   string `json:"bot_id"`
		Enabled bool   `json:"enabled"`
	}
	if json.NewDecoder(req.Body).Decode(&body) != nil || body.BotID == "" {
		BadRequestResponse(w, "bot_id required")
		return
	}
	ctx, cancel := context.WithTimeout(req.Context(), 4*time.Second)
	defer cancel()
	var err error
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
	var body struct {
		BotID string `json:"bot_id"`
	}
	if json.NewDecoder(req.Body).Decode(&body) != nil || body.BotID == "" {
		BadRequestResponse(w, "bot_id required")
		return
	}
	ctx, cancel := context.WithTimeout(req.Context(), 4*time.Second)
	defer cancel()
	if err := r.plugins.Restart(ctx, mux.Vars(req)["id"], body.BotID); err != nil {
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
