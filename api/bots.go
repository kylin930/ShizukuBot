package api

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"

	"shizukubot/config"
	"shizukubot/core"
)

type BotInfo struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	AppID         string   `json:"app_id"`
	Enabled       bool     `json:"enabled"`
	Running       bool     `json:"running"`
	Mode          string   `json:"mode"`
	Plugins       []string `json:"plugins"`
	CommandPrefix string   `json:"command_prefix"`
	AdminUsers    []string `json:"admin_users"`
}

func (r *Router) GetBots(w http.ResponseWriter, req *http.Request) {
	bots := r.botManager.GetAllBots()
	result := make([]BotInfo, 0, len(bots))
	for _, bot := range bots {
		result = append(result, BotInfo{
			ID:            bot.ID,
			Name:          bot.Name,
			AppID:         bot.AppID,
			Enabled:       bot.Enabled,
			Running:       bot.IsRunning(),
			Mode:          bot.Mode,
			Plugins:       bot.PluginIDs,
			CommandPrefix: bot.CommandPrefix,
			AdminUsers:    bot.AdminUsers,
		})
	}
	SuccessResponse(w, result)
}

func (r *Router) GetBot(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	id := vars["id"]

	bot, exists := r.botManager.GetBot(id)
	if !exists {
		NotFoundResponse(w, "bot not found")
		return
	}

	result := BotInfo{
		ID:            bot.ID,
		Name:          bot.Name,
		AppID:         bot.AppID,
		Enabled:       bot.Enabled,
		Running:       bot.IsRunning(),
		Mode:          bot.Mode,
		Plugins:       bot.PluginIDs,
		CommandPrefix: bot.CommandPrefix,
		AdminUsers:    bot.AdminUsers,
	}
	SuccessResponse(w, result)
}

func (r *Router) CreateBot(w http.ResponseWriter, req *http.Request) {
	var reqBody struct {
		ID            string   `json:"id"`
		Name          string   `json:"name"`
		AppID         string   `json:"app_id"`
		AppSecret     string   `json:"app_secret"`
		Enabled       bool     `json:"enabled"`
		Mode          string   `json:"mode"`
		Plugins       []string `json:"plugins"`
		CommandPrefix string   `json:"command_prefix"`
		AdminUsers    []string `json:"admin_users"`
	}

	if err := json.NewDecoder(req.Body).Decode(&reqBody); err != nil {
		BadRequestResponse(w, "invalid request body")
		return
	}

	if reqBody.ID == "" || reqBody.AppID == "" || reqBody.AppSecret == "" {
		BadRequestResponse(w, "id, app_id, and app_secret are required")
		return
	}

	mode := reqBody.Mode
	if mode == "" {
		mode = "websocket"
	}

	cfg := config.BotConfig{
		ID:            reqBody.ID,
		Name:          reqBody.Name,
		AppID:         reqBody.AppID,
		AppSecret:     reqBody.AppSecret,
		Enabled:       reqBody.Enabled,
		Mode:          mode,
		Plugins:       reqBody.Plugins,
		CommandPrefix: reqBody.CommandPrefix,
		AdminUsers:    reqBody.AdminUsers,
	}

	bot, err := core.NewBot(cfg)
	if err != nil {
		BadRequestResponse(w, err.Error())
		return
	}

	if err := r.botManager.AddBot(bot); err != nil {
		BadRequestResponse(w, err.Error())
		return
	}
	bot.Engine = r.engine

	r.config.Bots = append(r.config.Bots, cfg)
	if err := config.SaveConfig(r.configPath, r.config); err != nil {
		InternalErrorResponse(w, "failed to save config")
		return
	}

	result := BotInfo{
		ID:            bot.ID,
		Name:          bot.Name,
		AppID:         bot.AppID,
		Enabled:       bot.Enabled,
		Running:       bot.IsRunning(),
		Mode:          bot.Mode,
		Plugins:       bot.PluginIDs,
		CommandPrefix: bot.CommandPrefix,
		AdminUsers:    bot.AdminUsers,
	}
	SuccessResponse(w, result)
}

func (r *Router) UpdateBot(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	id := vars["id"]

	var reqBody struct {
		Name          string   `json:"name"`
		AppID         string   `json:"app_id"`
		AppSecret     string   `json:"app_secret"`
		Enabled       *bool    `json:"enabled"`
		Mode          string   `json:"mode"`
		Plugins       []string `json:"plugins"`
		CommandPrefix string   `json:"command_prefix"`
		AdminUsers    []string `json:"admin_users"`
	}

	if err := json.NewDecoder(req.Body).Decode(&reqBody); err != nil {
		BadRequestResponse(w, "invalid request body")
		return
	}

	bot, exists := r.botManager.GetBot(id)
	if !exists {
		NotFoundResponse(w, "bot not found")
		return
	}

	if reqBody.Name != "" {
		bot.Name = reqBody.Name
	}
	if reqBody.AppID != "" {
		bot.AppID = reqBody.AppID
	}
	if reqBody.AppSecret != "" {
		bot.AppSecret = reqBody.AppSecret
	}
	if reqBody.Enabled != nil {
		bot.Enabled = *reqBody.Enabled
	}
	if reqBody.Mode != "" {
		bot.Mode = reqBody.Mode
	}
	// Plugins 字段：nil 表示不修改，非nil（包括空切片）表示设置为该值
	if reqBody.Plugins != nil {
		bot.PluginIDs = reqBody.Plugins
	}
	if reqBody.CommandPrefix != "" {
		bot.CommandPrefix = reqBody.CommandPrefix
	}
	// AdminUsers 字段：nil 表示不修改，非nil（包括空切片）表示设置为该值
	if reqBody.AdminUsers != nil {
		bot.AdminUsers = reqBody.AdminUsers
	}

	for i, cfg := range r.config.Bots {
		if cfg.ID == id {
			if reqBody.Name != "" {
				r.config.Bots[i].Name = reqBody.Name
			}
			if reqBody.AppID != "" {
				r.config.Bots[i].AppID = reqBody.AppID
			}
			if reqBody.AppSecret != "" {
				r.config.Bots[i].AppSecret = reqBody.AppSecret
			}
			if reqBody.Enabled != nil {
				r.config.Bots[i].Enabled = *reqBody.Enabled
			}
			if reqBody.Mode != "" {
				r.config.Bots[i].Mode = reqBody.Mode
			}
			if reqBody.Plugins != nil {
				r.config.Bots[i].Plugins = reqBody.Plugins
			}
			if reqBody.CommandPrefix != "" {
				r.config.Bots[i].CommandPrefix = reqBody.CommandPrefix
			}
			if reqBody.AdminUsers != nil {
				r.config.Bots[i].AdminUsers = reqBody.AdminUsers
			}
			break
		}
	}

	if err := config.SaveConfig(r.configPath, r.config); err != nil {
		InternalErrorResponse(w, "failed to save config")
		return
	}

	result := BotInfo{
		ID:            bot.ID,
		Name:          bot.Name,
		AppID:         bot.AppID,
		Enabled:       bot.Enabled,
		Running:       bot.IsRunning(),
		Mode:          bot.Mode,
		Plugins:       bot.PluginIDs,
		CommandPrefix: bot.CommandPrefix,
		AdminUsers:    bot.AdminUsers,
	}
	SuccessResponse(w, result)
}

func (r *Router) DeleteBot(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	id := vars["id"]

	if err := r.botManager.RemoveBot(id); err != nil {
		NotFoundResponse(w, err.Error())
		return
	}

	for i, cfg := range r.config.Bots {
		if cfg.ID == id {
			r.config.Bots = append(r.config.Bots[:i], r.config.Bots[i+1:]...)
			break
		}
	}

	if err := config.SaveConfig(r.configPath, r.config); err != nil {
		InternalErrorResponse(w, "failed to save config")
		return
	}

	SuccessResponse(w, nil)
}

func (r *Router) StartBot(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	id := vars["id"]

	bot, exists := r.botManager.GetBot(id)
	if !exists {
		NotFoundResponse(w, "bot not found")
		return
	}

	if err := bot.Start(); err != nil {
		BadRequestResponse(w, err.Error())
		return
	}

	// 更新配置中的 enabled 状态
	for i := range r.config.Bots {
		if r.config.Bots[i].ID == id {
			r.config.Bots[i].Enabled = true
			break
		}
	}
	config.SaveConfig(r.configPath, r.config)

	SuccessResponse(w, BotInfo{
		ID:            bot.ID,
		Name:          bot.Name,
		AppID:         bot.AppID,
		Enabled:       bot.Enabled,
		Running:       bot.IsRunning(),
		Mode:          bot.Mode,
		Plugins:       bot.PluginIDs,
		CommandPrefix: bot.CommandPrefix,
		AdminUsers:    bot.AdminUsers,
	})
}

func (r *Router) StopBot(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	id := vars["id"]

	bot, exists := r.botManager.GetBot(id)
	if !exists {
		NotFoundResponse(w, "bot not found")
		return
	}

	bot.Stop()

	// 更新配置中的 enabled 状态
	for i := range r.config.Bots {
		if r.config.Bots[i].ID == id {
			r.config.Bots[i].Enabled = false
			break
		}
	}
	config.SaveConfig(r.configPath, r.config)

	SuccessResponse(w, BotInfo{
		ID:            bot.ID,
		Name:          bot.Name,
		AppID:         bot.AppID,
		Enabled:       bot.Enabled,
		Running:       bot.IsRunning(),
		Mode:          bot.Mode,
		Plugins:       bot.PluginIDs,
		CommandPrefix: bot.CommandPrefix,
		AdminUsers:    bot.AdminUsers,
	})
}