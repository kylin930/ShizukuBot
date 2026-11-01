package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/gorilla/mux"
	"net/http"
	"shizukubot/plugins"
	"time"
)

type PluginInfo struct {
	plugins.Manifest
	Enabled   bool             `json:"enabled"`
	Instances []plugins.Status `json:"instances"`
	Config    map[string]any   `json:"config,omitempty"`
	Trusted   bool             `json:"trusted"`
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
		out.Trusted = settings.Trusted
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
func (r *Router) CreatePlugin(w http.ResponseWriter, req *http.Request) {
	var body struct {
		Package     string `json:"package_base64"`
		RestoreData bool   `json:"restore_data"`
	}
	if json.NewDecoder(req.Body).Decode(&body) != nil {
		BadRequestResponse(w, "invalid package request")
		return
	}
	data, err := base64.StdEncoding.DecodeString(body.Package)
	if err != nil {
		BadRequestResponse(w, "invalid package encoding")
		return
	}
	p, err := r.plugins.InstallPackage(data, body.RestoreData)
	if err != nil {
		BadRequestResponse(w, "package rejected: "+err.Error())
		return
	}
	SuccessResponse(w, r.pluginInfo(p, ""))
}
func (r *Router) UpdatePlugin(w http.ResponseWriter, req *http.Request) {
	id := mux.Vars(req)["id"]
	var body struct {
		BotID   string         `json:"bot_id"`
		Config  map[string]any `json:"config"`
		Trusted *bool          `json:"trusted"`
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
	if body.Trusted != nil {
		if !*body.Trusted {
			ctx, cancel := context.WithTimeout(req.Context(), 3*time.Second)
			defer cancel()
			if err := r.plugins.Disable(ctx, id, body.BotID); err != nil {
				InternalErrorResponse(w, "disable failed")
				return
			}
		}
		if err := r.plugins.Trust(id, body.BotID, *body.Trusted); err != nil {
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
		Trust   bool   `json:"trust"`
	}
	if json.NewDecoder(req.Body).Decode(&body) != nil || body.BotID == "" {
		BadRequestResponse(w, "bot_id required")
		return
	}
	ctx, cancel := context.WithTimeout(req.Context(), 4*time.Second)
	defer cancel()
	var err error
	if body.Enabled {
		err = r.plugins.Enable(ctx, id, body.BotID, body.Trust)
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
	ctx, cancel := context.WithTimeout(req.Context(), 4*time.Second)
	defer cancel()
	if err := r.plugins.Remove(ctx, mux.Vars(req)["id"]); err != nil {
		BadRequestResponse(w, "plugin removal failed")
		return
	}
	SuccessResponse(w, nil)
}
func (r *Router) ExportPlugin(w http.ResponseWriter, req *http.Request) {
	id := mux.Vars(req)["id"]
	var data []byte
	var err error
	if req.URL.Query().Get("backup") == "1" {
		data, err = r.plugins.ExportBackup(id)
	} else {
		data, err = r.plugins.Store.Export(id)
	}
	if err != nil {
		BadRequestResponse(w, "package export failed")
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+id+`.zip"`)
	w.Write(data)
}
func (r *Router) Close(ctx context.Context) error { return r.plugins.Close(ctx) }
