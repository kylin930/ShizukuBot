package api

import (
	"net/http"
	"strconv"

	"shizukubot/core"
)

type LogResponse struct {
	Timestamp string `json:"timestamp"`
	BotID     string `json:"bot_id"`
	Level     string `json:"level"`
	Message   string `json:"message"`
}

func (r *Router) GetLogs(w http.ResponseWriter, req *http.Request) {
	botID := req.URL.Query().Get("botID")
	levelStr := req.URL.Query().Get("level")
	limitStr := req.URL.Query().Get("limit")

	limit := 100
	if limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	var minLevel core.LogLevel
	switch levelStr {
	case "debug":
		minLevel = core.LogLevelDebug
	case "info":
		minLevel = core.LogLevelInfo
	case "warn":
		minLevel = core.LogLevelWarn
	case "error":
		minLevel = core.LogLevelError
	default:
		minLevel = core.LogLevelDebug
	}

	logManager := r.botManager.GetLogManager()
	rawLogs := logManager.GetHistory(botID, limit)

	result := make([]LogResponse, 0, len(rawLogs))
	for _, log := range rawLogs {
		if log.Level >= minLevel {
			result = append(result, LogResponse{
				Timestamp: log.Timestamp.Format("2006-01-02 15:04:05"),
				BotID:     log.BotID,
				Level:     log.Level.String(),
				Message:   log.Message,
			})
		}
	}

	SuccessResponse(w, result)
}