package api

import (
	"encoding/json"
	"net/http"

	"shizukubot/config"
	"shizukubot/core"
	"shizukubot/webui"
)

type ConfigResponse struct {
	WebUI struct {
		Enabled  bool   `json:"enabled"`
		Port     int    `json:"port"`
		Password string `json:"password"`
	} `json:"webui"`
	Bots            []BotInfo `json:"bots"`
	MarketAPI       string    `json:"market_api"`
	BuiltinCommands struct {
		Help    string `json:"help"`
		Status  string `json:"status"`
		Shizuku string `json:"shizuku"`
	} `json:"builtin_commands"`
}

func (r *Router) GetConfig(w http.ResponseWriter, req *http.Request) {
	snapshot := r.configSnapshot()
	result := ConfigResponse{}
	result.WebUI.Enabled = snapshot.WebUI.Enabled
	result.WebUI.Port = snapshot.WebUI.Port
	// Session signing secret is write-only.
	result.MarketAPI = snapshot.MarketAPI
	result.BuiltinCommands.Help = snapshot.BuiltinCommands.Help
	result.BuiltinCommands.Status = snapshot.BuiltinCommands.Status
	result.BuiltinCommands.Shizuku = snapshot.BuiltinCommands.Shizuku

	bots := r.botManager.GetAllBots()
	result.Bots = make([]BotInfo, 0, len(bots))
	for _, bot := range bots {
		result.Bots = append(result.Bots, BotInfo{
			ID:      bot.ID,
			Name:    bot.Name,
			AppID:   bot.AppID,
			Enabled: bot.IsEnabled(),
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
		MarketAPI       string `json:"market_api"`
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

	if reqBody.WebUI.Port < 1 || reqBody.WebUI.Port > 65535 {
		BadRequestResponse(w, "invalid port")
		return
	}
	err := r.changeConfig(func(snapshot *config.Config) {
		snapshot.WebUI.Enabled = reqBody.WebUI.Enabled
		snapshot.WebUI.Port = reqBody.WebUI.Port
		if reqBody.WebUI.Password != "" {
			snapshot.WebUI.Password = reqBody.WebUI.Password
		}
		snapshot.MarketAPI = reqBody.MarketAPI
		snapshot.BuiltinCommands = config.BuiltinCommandsConfig{Help: reqBody.BuiltinCommands.Help, Status: reqBody.BuiltinCommands.Status, Shizuku: reqBody.BuiltinCommands.Shizuku}
	})
	if err != nil {
		InternalErrorResponse(w, "failed to save config")
		return
	}
	if reqBody.WebUI.Password != "" {
		webui.InitSessionStore(reqBody.WebUI.Password)
	}
	core.SetCommandTemplates(&core.CommandTemplates{Help: reqBody.BuiltinCommands.Help, Status: reqBody.BuiltinCommands.Status, Shizuku: reqBody.BuiltinCommands.Shizuku})
	r.GetConfig(w, req)
}
