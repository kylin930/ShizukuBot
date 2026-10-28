package api

import (
	"encoding/json"
	"net/http"

	"shizukubot/config"
	"shizukubot/core"
)

type ConfigResponse struct {
	WebUI struct {
		Enabled  bool   `json:"enabled"`
		Port     int    `json:"port"`
		Password string `json:"password"`
	} `json:"webui"`
	Bots      []BotInfo `json:"bots"`
	MarketAPI string    `json:"market_api"`
	BuiltinCommands struct {
		Help    string `json:"help"`
		Status  string `json:"status"`
		Shizuku string `json:"shizuku"`
	} `json:"builtin_commands"`
}

func (r *Router) GetConfig(w http.ResponseWriter, req *http.Request) {
	result := ConfigResponse{}
	result.WebUI.Enabled = r.config.WebUI.Enabled
	result.WebUI.Port = r.config.WebUI.Port
	result.WebUI.Password = r.config.WebUI.Password
	result.MarketAPI = r.config.MarketAPI
	result.BuiltinCommands.Help = r.config.BuiltinCommands.Help
	result.BuiltinCommands.Status = r.config.BuiltinCommands.Status
	result.BuiltinCommands.Shizuku = r.config.BuiltinCommands.Shizuku

	bots := r.botManager.GetAllBots()
	result.Bots = make([]BotInfo, 0, len(bots))
	for _, bot := range bots {
		result.Bots = append(result.Bots, BotInfo{
			ID:      bot.ID,
			Name:    bot.Name,
			AppID:   bot.AppID,
			Enabled: bot.Enabled,
		})
	}

	SuccessResponse(w, result)
}

func (r *Router) UpdateConfig(w http.ResponseWriter, req *http.Request) {
	var reqBody struct {
		WebUI struct {
			Enabled  bool   `json:"enabled"`
			Port     int    `json:"port"`
			Password string `json:"password"`
		} `json:"webui"`
		MarketAPI string `json:"market_api"`
		BuiltinCommands struct {
			Help    string `json:"help"`
			Status  string `json:"status"`
			Shizuku string `json:"shizuku"`
		} `json:"builtin_commands"`
	}

	if err := json.NewDecoder(req.Body).Decode(&reqBody); err != nil {
		BadRequestResponse(w, "invalid request body")
		return
	}

	r.config.WebUI.Enabled = reqBody.WebUI.Enabled
	r.config.WebUI.Port = reqBody.WebUI.Port
	r.config.WebUI.Password = reqBody.WebUI.Password
	r.config.MarketAPI = reqBody.MarketAPI
	r.config.BuiltinCommands.Help = reqBody.BuiltinCommands.Help
	r.config.BuiltinCommands.Status = reqBody.BuiltinCommands.Status
	r.config.BuiltinCommands.Shizuku = reqBody.BuiltinCommands.Shizuku

	// 同步到运行中的内置指令模板，使其立即生效
	core.GlobalCommandTemplates = &core.CommandTemplates{
		Help:    reqBody.BuiltinCommands.Help,
		Status:  reqBody.BuiltinCommands.Status,
		Shizuku: reqBody.BuiltinCommands.Shizuku,
	}

	if err := config.SaveConfig(r.configPath, r.config); err != nil {
		InternalErrorResponse(w, "failed to save config")
		return
	}

	result := ConfigResponse{}
	result.WebUI.Enabled = r.config.WebUI.Enabled
	result.WebUI.Port = r.config.WebUI.Port
	result.WebUI.Password = r.config.WebUI.Password
	result.MarketAPI = r.config.MarketAPI
	result.BuiltinCommands.Help = r.config.BuiltinCommands.Help
	result.BuiltinCommands.Status = r.config.BuiltinCommands.Status
	result.BuiltinCommands.Shizuku = r.config.BuiltinCommands.Shizuku

	bots := r.botManager.GetAllBots()
	result.Bots = make([]BotInfo, 0, len(bots))
	for _, bot := range bots {
		result.Bots = append(result.Bots, BotInfo{
			ID:      bot.ID,
			Name:    bot.Name,
			AppID:   bot.AppID,
			Enabled: bot.Enabled,
		})
	}

	SuccessResponse(w, result)
}