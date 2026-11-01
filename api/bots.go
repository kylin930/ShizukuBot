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
			Enabled:       bot.IsEnabled(),
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
		Enabled:       bot.IsEnabled(),
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

	r.config.Bots = append(r.config.Bots, cfg)
	if err := config.SaveConfig(r.configPath, r.config); err != nil {
		InternalErrorResponse(w, "failed to save config")
		return
	}

	result := BotInfo{
		ID:            bot.ID,
		Name:          bot.Name,
		AppID:         bot.AppID,
		Enabled:       bot.IsEnabled(),
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

	var next config.BotConfig
	for _, cfg := range r.config.Bots {
		if cfg.ID == id {
			next = cfg
			break
		}
	}
	if next.ID == "" {
		NotFoundResponse(w, "bot config not found")
		return
	}
	if reqBody.Name != "" {
		next.Name = reqBody.Name
	}
	if reqBody.AppID != "" {
		next.AppID = reqBody.AppID
	}
	if reqBody.AppSecret != "" {
		next.AppSecret = reqBody.AppSecret
	}
	if reqBody.Mode != "" {
		next.Mode = reqBody.Mode
	}
	if reqBody.Enabled != nil {
		next.Enabled = *reqBody.Enabled
	}
	if reqBody.Plugins != nil {
		next.Plugins = reqBody.Plugins
	}
	if reqBody.CommandPrefix != "" {
		next.CommandPrefix = reqBody.CommandPrefix
	}
	if reqBody.AdminUsers != nil {
		next.AdminUsers = reqBody.AdminUsers
	}
	replacement, err := core.NewBot(next)
	if err != nil {
		BadRequestResponse(w, "invalid bot configuration")
		return
	}
	wasRunning := bot.IsRunning()
	if err = r.botManager.ReplaceBot(replacement); err != nil {
		InternalErrorResponse(w, "bot replacement failed")
		return
	}
	bot = replacement

	for i, cfg := range r.config.Bots {
		if cfg.ID == id {
			r.config.Bots[i] = next
			break
		}
	}
	if err := config.SaveConfig(r.configPath, r.config); err != nil {
		InternalErrorResponse(w, "failed to save config")
		return
	}
	if wasRunning && next.Enabled {
		if err = bot.Start(); err != nil {
			BadRequestResponse(w, "configuration saved; bot restart failed")
			return
		}
	}

	result := BotInfo{
		ID:            bot.ID,
		Name:          bot.Name,
		AppID:         bot.AppID,
		Enabled:       bot.IsEnabled(),
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
	if err := config.SaveConfig(r.configPath, r.config); err != nil {
		InternalErrorResponse(w, "failed to save config")
		return
	}

	SuccessResponse(w, BotInfo{
		ID:            bot.ID,
		Name:          bot.Name,
		AppID:         bot.AppID,
		Enabled:       bot.IsEnabled(),
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
	if err := config.SaveConfig(r.configPath, r.config); err != nil {
		InternalErrorResponse(w, "failed to save config")
		return
	}

	SuccessResponse(w, BotInfo{
		ID:            bot.ID,
		Name:          bot.Name,
		AppID:         bot.AppID,
		Enabled:       bot.IsEnabled(),
		Running:       bot.IsRunning(),
		Mode:          bot.Mode,
		Plugins:       bot.PluginIDs,
		CommandPrefix: bot.CommandPrefix,
		AdminUsers:    bot.AdminUsers,
	})
}
