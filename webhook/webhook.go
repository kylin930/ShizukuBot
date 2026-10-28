package webhook

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/tencent-connect/botgo/interaction/webhook"
	"github.com/tencent-connect/botgo/token"

	"shizukubot/core"
)

type Handler struct {
	botManager *core.BotManager
}

func NewHandler(botManager *core.BotManager) *Handler {
	return &Handler{
		botManager: botManager,
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	botID := vars["botID"]
	if botID == "" {
		http.Error(w, "botID is required", http.StatusBadRequest)
		return
	}

	bot, exists := h.botManager.GetBot(botID)
	if !exists {
		http.Error(w, "bot not found", http.StatusNotFound)
		return
	}

	credentials := &token.QQBotCredentials{
		AppID:     bot.AppID,
		AppSecret: bot.AppSecret,
	}

	webhook.HTTPHandler(w, r, credentials)
}