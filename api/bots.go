package api

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"

	"shizukubot/config"
	"shizukubot/core"
)

type BotInfo struct {
	Diagnostics   core.QQDiagnostics `json:"diagnostics"`
	ID            string             `json:"id"`
	Name          string             `json:"name"`
	AppID         string             `json:"app_id"`
	Enabled       bool               `json:"enabled"`
	Running       bool               `json:"running"`
	Mode          string             `json:"mode"`
	Plugins       []string           `json:"plugins"`
	CommandPrefix string             `json:"command_prefix"`
	AdminUsers    []string           `json:"admin_users"`
}

func (r *Router) GetBots(w http.ResponseWriter, req *http.Request) {
	bots := r.botManager.GetAllBots()
	result := make([]BotInfo, 0, len(bots))
	for _, bot := range bots {
		result = append(result, botInfo(bot))
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

	result := botInfo(bot)
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
		mode = "webhook"
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

	if err := r.changeConfig(func(c *config.Config) { c.Bots = append(c.Bots, cfg) }); err != nil {
		_ = r.botManager.RemoveBot(bot.ID)
		InternalErrorResponse(w, "failed to save config")
		return
	}

	result := botInfo(bot)
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
	for _, cfg := range r.configSnapshot().Bots {
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

	if err := r.changeConfig(func(c *config.Config) {
		for i, cfg := range c.Bots {
			if cfg.ID == id {
				c.Bots[i] = next
				break
			}
		}
	}); err != nil {
		InternalErrorResponse(w, "failed to save config")
		return
	}

	if wasRunning && next.Enabled {
		if err = bot.Start(); err != nil {
			BadRequestResponse(w, "configuration saved; bot restart failed")
			return
		}
	}

	result := botInfo(bot)
	SuccessResponse(w, result)
}

func (r *Router) DeleteBot(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	id := vars["id"]

	if err := r.botManager.RemoveBot(id); err != nil {
		NotFoundResponse(w, err.Error())
		return
	}

	if err := r.changeConfig(func(c *config.Config) {
		for i, cfg := range c.Bots {
			if cfg.ID == id {
				c.Bots = append(c.Bots[:i], c.Bots[i+1:]...)
				break
			}
		}
	}); err != nil {
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

	if err := r.changeConfig(func(c *config.Config) {
		for i := range c.Bots {
			if c.Bots[i].ID == id {
				c.Bots[i].Enabled = true
				break
			}
		}
	}); err != nil {
		InternalErrorResponse(w, "failed to save config")
		return
	}

	SuccessResponse(w, botInfo(bot))
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

	if err := r.changeConfig(func(c *config.Config) {
		for i := range c.Bots {
			if c.Bots[i].ID == id {
				c.Bots[i].Enabled = false
				break
			}
		}
	}); err != nil {
		InternalErrorResponse(w, "failed to save config")
		return
	}

	SuccessResponse(w, botInfo(bot))
}

func botInfo(b *core.Bot) BotInfo {
	c := b.ConfigSnapshot()
	return BotInfo{Diagnostics: b.Diagnostics(), ID: c.ID, Name: c.Name, AppID: c.AppID, Enabled: c.Enabled, Running: b.IsRunning(), Mode: c.Mode, Plugins: c.Plugins, CommandPrefix: c.CommandPrefix, AdminUsers: c.AdminUsers}
}
