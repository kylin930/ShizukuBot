package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"

	"shizukubot/plugin"
)

type PluginExport struct {
	ID           string           `json:"id"`
	Name         string           `json:"name"`
	Description  string           `json:"description"`
	BotID        string           `json:"bot_id"`
	Graph        json.RawMessage  `json:"graph"`
	CustomNodes  json.RawMessage  `json:"custom_nodes,omitempty"`
}

type PluginInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
	BotID       string `json:"bot_id"`
	CreatedAt   int64  `json:"created_at"`
	UpdatedAt   int64  `json:"updated_at"`
}

type PluginDetail struct {
	ID           string           `json:"id"`
	Name         string           `json:"name"`
	Description  string           `json:"description"`
	Enabled      bool             `json:"enabled"`
	BotID        string           `json:"bot_id"`
	Graph        json.RawMessage  `json:"graph"`
	CustomNodes  json.RawMessage  `json:"custom_nodes,omitempty"`
	CreatedAt    int64            `json:"created_at"`
	UpdatedAt    int64            `json:"updated_at"`
}

func (r *Router) GetPlugins(w http.ResponseWriter, req *http.Request) {
	plugins := r.pluginStore.List()
	result := make([]PluginInfo, 0, len(plugins))
	for _, p := range plugins {
		result = append(result, PluginInfo{
			ID:          p.ID,
			Name:        p.Name,
			Description: p.Description,
			Enabled:     p.Enabled,
			BotID:       p.BotID,
			CreatedAt:   p.CreatedAt,
			UpdatedAt:   p.UpdatedAt,
		})
	}
	SuccessResponse(w, result)
}

func (r *Router) GetPlugin(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	id := vars["id"]

	p, ok := r.pluginStore.Get(id)
	if !ok {
		NotFoundResponse(w, "plugin not found")
		return
	}

	result := PluginDetail{
		ID:          p.ID,
		Name:        p.Name,
		Description: p.Description,
		Enabled:     p.Enabled,
		BotID:       p.BotID,
		Graph:       p.Graph,
		CustomNodes: p.CustomNodes,
		CreatedAt:   p.CreatedAt,
		UpdatedAt:   p.UpdatedAt,
	}
	SuccessResponse(w, result)
}

func (r *Router) CreatePlugin(w http.ResponseWriter, req *http.Request) {
	var reqBody struct {
		Name         string           `json:"name"`
		Description  string           `json:"description"`
		BotID        string           `json:"bot_id"`
		Graph        json.RawMessage  `json:"graph"`
		CustomNodes  json.RawMessage  `json:"custom_nodes"`
	}

	if err := json.NewDecoder(req.Body).Decode(&reqBody); err != nil {
		BadRequestResponse(w, "invalid request body")
		return
	}

	if reqBody.Name == "" {
		BadRequestResponse(w, "name is required")
		return
	}

	if reqBody.Graph == nil {
		reqBody.Graph = json.RawMessage(`{"nodes":[],"links":[]}`)
	}

	now := time.Now().Unix()
	id := "plugin_" + strconv.FormatInt(now, 10)

	p := &plugin.Plugin{
		ID:           id,
		Name:         reqBody.Name,
		Description:  reqBody.Description,
		Enabled:      false,
		BotID:        reqBody.BotID,
		Graph:        reqBody.Graph,
		CustomNodes:  reqBody.CustomNodes,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := r.pluginStore.Save(p); err != nil {
		InternalErrorResponse(w, "failed to save plugin")
		return
	}

	result := PluginDetail{
		ID:          p.ID,
		Name:        p.Name,
		Description: p.Description,
		Enabled:     p.Enabled,
		BotID:       p.BotID,
		Graph:       p.Graph,
		CustomNodes: p.CustomNodes,
		CreatedAt:   p.CreatedAt,
		UpdatedAt:   p.UpdatedAt,
	}
	SuccessResponse(w, result)
}

func (r *Router) UpdatePlugin(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	id := vars["id"]

	p, ok := r.pluginStore.Get(id)
	if !ok {
		NotFoundResponse(w, "plugin not found")
		return
	}

	var reqBody struct {
		Name         string           `json:"name"`
		Description  string           `json:"description"`
		Enabled      bool             `json:"enabled"`
		BotID        string           `json:"bot_id"`
		Graph        json.RawMessage  `json:"graph"`
		CustomNodes  json.RawMessage  `json:"custom_nodes"`
	}

	if err := json.NewDecoder(req.Body).Decode(&reqBody); err != nil {
		BadRequestResponse(w, "invalid request body")
		return
	}

	if reqBody.Name != "" {
		p.Name = reqBody.Name
	}
	if reqBody.Description != "" {
		p.Description = reqBody.Description
	}
	p.Enabled = reqBody.Enabled
	if reqBody.BotID != "" {
		p.BotID = reqBody.BotID
	}
	if reqBody.Graph != nil {
		p.Graph = reqBody.Graph
	}
	if reqBody.CustomNodes != nil {
		p.CustomNodes = reqBody.CustomNodes
	}
	p.UpdatedAt = time.Now().Unix()

	if err := r.pluginStore.Save(p); err != nil {
		InternalErrorResponse(w, "failed to save plugin")
		return
	}

	result := PluginDetail{
		ID:          p.ID,
		Name:        p.Name,
		Description: p.Description,
		Enabled:     p.Enabled,
		BotID:       p.BotID,
		Graph:       p.Graph,
		CustomNodes: p.CustomNodes,
		CreatedAt:   p.CreatedAt,
		UpdatedAt:   p.UpdatedAt,
	}
	SuccessResponse(w, result)
}

func (r *Router) DeletePlugin(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	id := vars["id"]

	if err := r.pluginStore.Delete(id); err != nil {
		NotFoundResponse(w, err.Error())
		return
	}

	SuccessResponse(w, nil)
}

func (r *Router) TogglePlugin(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	id := vars["id"]

	p, ok := r.pluginStore.Get(id)
	if !ok {
		NotFoundResponse(w, "plugin not found")
		return
	}

	p.Enabled = !p.Enabled
	p.UpdatedAt = time.Now().Unix()

	if err := r.pluginStore.Save(p); err != nil {
		InternalErrorResponse(w, "failed to save plugin")
		return
	}

	result := PluginInfo{
		ID:          p.ID,
		Name:        p.Name,
		Description: p.Description,
		Enabled:     p.Enabled,
		BotID:       p.BotID,
		CreatedAt:   p.CreatedAt,
		UpdatedAt:   p.UpdatedAt,
	}
	SuccessResponse(w, result)
}

func (r *Router) SavePluginGraph(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	id := vars["id"]

	p, ok := r.pluginStore.Get(id)
	if !ok {
		NotFoundResponse(w, "plugin not found")
		return
	}

	var graph json.RawMessage
	if err := json.NewDecoder(req.Body).Decode(&graph); err != nil {
		BadRequestResponse(w, "invalid graph data")
		return
	}

	p.Graph = graph
	p.UpdatedAt = time.Now().Unix()

	if err := r.pluginStore.Save(p); err != nil {
		InternalErrorResponse(w, "failed to save plugin")
		return
	}

	SuccessResponse(w, map[string]interface{}{
		"id":         p.ID,
		"updated_at": p.UpdatedAt,
	})
}

func (r *Router) ExportPlugin(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	id := vars["id"]

	p, ok := r.pluginStore.Get(id)
	if !ok {
		NotFoundResponse(w, "plugin not found")
		return
	}

	exportData := PluginExport{
		ID:           p.ID,
		Name:         p.Name,
		Description:  p.Description,
		BotID:        p.BotID,
		Graph:        p.Graph,
		CustomNodes:  p.CustomNodes,
	}

	data, err := json.MarshalIndent(exportData, "", "  ")
	if err != nil {
		InternalErrorResponse(w, "failed to marshal plugin")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+p.Name+"_"+p.ID+".json\"")
	w.Write(data)
}