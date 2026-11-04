package api

import (
	"encoding/json"
	"errors"
	"net/http"
	stats "shizukubot/storage/stats"

	"shizukubot/config"
	"shizukubot/core"
	"shizukubot/webui"
)

type ConfigResponse struct {
	Revision   string        `json:"revision"`
	Statistics stats.Options `json:"statistics"`
	WebUI      struct {
		Appearance config.Appearance `json:"appearance"`
		Enabled    bool              `json:"enabled"`
		Port       int               `json:"port"`
		Password   string            `json:"password"`
	} `json:"webui"`
	Bots            []BotInfo `json:"bots"`
	MarketAPI       string    `json:"market_api"`
	BuiltinCommands struct {
		HelpEnabled       bool   `json:"help_enabled"`
		StatusEnabled     bool   `json:"status_enabled"`
		ShowShizukuInHelp bool   `json:"show_shizuku_in_help"`
		Help              string `json:"help"`
		Status            string `json:"status"`
		Shizuku           string `json:"shizuku"`
	} `json:"builtin_commands"`
}

func (r *Router) GetConfig(w http.ResponseWriter, req *http.Request) {
	r.configMu.RLock()
	snapshot := cloneConfig(r.config)
	revision := r.configRevision
	r.configMu.RUnlock()
	result := ConfigResponse{}
	result.Revision = revision
	result.Statistics = snapshot.Statistics
	result.WebUI.Appearance = snapshot.WebUI.Appearance
	result.WebUI.Enabled = snapshot.WebUI.Enabled
	result.WebUI.Port = snapshot.WebUI.Port
	// Session signing secret is write-only.
	result.MarketAPI = snapshot.MarketAPI
	result.BuiltinCommands.HelpEnabled = config.BuiltinEnabled(snapshot.BuiltinCommands.HelpEnabled)
	result.BuiltinCommands.StatusEnabled = config.BuiltinEnabled(snapshot.BuiltinCommands.StatusEnabled)
	result.BuiltinCommands.ShowShizukuInHelp = config.BuiltinEnabled(snapshot.BuiltinCommands.ShowShizukuInHelp)
	result.BuiltinCommands.Help = snapshot.BuiltinCommands.Help
	result.BuiltinCommands.Status = snapshot.BuiltinCommands.Status
	result.BuiltinCommands.Shizuku = snapshot.BuiltinCommands.Shizuku

	bots := r.botManager.GetAllBots()
	result.Bots = make([]BotInfo, 0, len(bots))
	for _, bot := range bots {
		result.Bots = append(result.Bots, botInfo(bot))
	}

	SuccessResponse(w, result)
}

func (r *Router) UpdateConfig(w http.ResponseWriter, req *http.Request) {
	var reqBody struct {
		Revision   string         `json:"revision"`
		Statistics *stats.Options `json:"statistics"`
		WebUI      struct {
			Appearance *config.Appearance `json:"appearance"`
			Enabled    bool               `json:"enabled"`
			Port       int                `json:"port"`
			Password   string             `json:"password"`
		} `json:"webui"`
		MarketAPI       string `json:"market_api"`
		BuiltinCommands struct {
			HelpEnabled       *bool  `json:"help_enabled"`
			StatusEnabled     *bool  `json:"status_enabled"`
			ShowShizukuInHelp *bool  `json:"show_shizuku_in_help"`
			Help              string `json:"help"`
			Status            string `json:"status"`
			Shizuku           string `json:"shizuku"`
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
	r.appearanceMu.Lock()
	defer r.appearanceMu.Unlock()
	oldBackground := r.configSnapshot().WebUI.Appearance.Background
	if reqBody.Statistics != nil {
		if store := r.botManager.StatsStore(); store != nil && store.Settings().Timezone != reqBody.Statistics.Timezone {
			BadRequestResponse(w, "统计时区不能与已有数据库不同")
			return
		}
		if err := reqBody.Statistics.Validate(); err != nil {
			BadRequestResponse(w, err.Error())
			return
		}
	}
	if reqBody.WebUI.Appearance != nil {
		if err := reqBody.WebUI.Appearance.Validate(); err != nil {
			BadRequestResponse(w, err.Error())
			return
		}
		if err := r.validateBackgroundFile(reqBody.WebUI.Appearance.Background); err != nil {
			BadRequestResponse(w, err.Error())
			return
		}
	}
	err := r.changeConfigChecked(reqBody.Revision, func(snapshot *config.Config) error {
		if reqBody.Statistics != nil {
			snapshot.Statistics = *reqBody.Statistics
		}
		if reqBody.WebUI.Appearance != nil {
			snapshot.WebUI.Appearance = *reqBody.WebUI.Appearance
		}

		snapshot.WebUI.Enabled = reqBody.WebUI.Enabled
		snapshot.WebUI.Port = reqBody.WebUI.Port
		if reqBody.WebUI.Password != "" {
			snapshot.WebUI.Password = reqBody.WebUI.Password
		}
		snapshot.MarketAPI = reqBody.MarketAPI
		snapshot.BuiltinCommands.Help = reqBody.BuiltinCommands.Help
		snapshot.BuiltinCommands.Status = reqBody.BuiltinCommands.Status
		snapshot.BuiltinCommands.Shizuku = reqBody.BuiltinCommands.Shizuku
		if reqBody.BuiltinCommands.HelpEnabled != nil {
			snapshot.BuiltinCommands.HelpEnabled = reqBody.BuiltinCommands.HelpEnabled
		}
		if reqBody.BuiltinCommands.StatusEnabled != nil {
			snapshot.BuiltinCommands.StatusEnabled = reqBody.BuiltinCommands.StatusEnabled
		}
		if reqBody.BuiltinCommands.ShowShizukuInHelp != nil {
			snapshot.BuiltinCommands.ShowShizukuInHelp = reqBody.BuiltinCommands.ShowShizukuInHelp
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, errConfigConflict) {
			ErrorResponse(w, 409, err.Error())
		} else {
			InternalErrorResponse(w, "配置保存失败")
		}
		return
	}
	if reqBody.Statistics != nil {
		if store := r.botManager.StatsStore(); store != nil {
			if err := store.Update(req.Context(), *reqBody.Statistics); err != nil {
				InternalErrorResponse(w, "配置已保存，统计设置应用失败："+err.Error())
				return
			}
		}
	}
	if next := r.configSnapshot().WebUI.Appearance.Background; oldBackground.Type == "upload" && oldBackground.Value != next.Value {
		if err := r.removeBackground(oldBackground.Value); err != nil {
			r.logManager.Error("", "old background cleanup failed: "+err.Error())
		}
	}
	if reqBody.WebUI.Password != "" {
		webui.InitSessionStore(reqBody.WebUI.Password)
	}
	core.ConfigureBuiltinCommands(r.configSnapshot().BuiltinCommands)
	r.GetConfig(w, req)
}
