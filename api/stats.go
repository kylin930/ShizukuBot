package api

import (
	"net/http"

	"github.com/gorilla/mux"
)

type StatsResponse struct {
	UserCount        int64 `json:"user_count"`
	GroupCount       int64 `json:"group_count"`
	TodayDAU         int64 `json:"today_dau"`
	QPS              int64 `json:"qps"`
	SentMessages     int64 `json:"sent_messages"`
	ReceivedMessages int64 `json:"received_messages"`
}

func (r *Router) GetStats(w http.ResponseWriter, req *http.Request) {
	stats := r.botManager.GetTotalStats()
	hourly := r.botManager.GetTodayHourlyStats()
	
	result := map[string]interface{}{
		"user_count":        stats.UserCount,
		"group_count":       stats.GroupCount,
		"today_dau":         stats.TodayDAU,
		"qps":               stats.QPS,
		"sent_messages":     stats.SentMessages,
		"received_messages": stats.ReceivedMessages,
		"hourly_stats":      hourly,
	}
	SuccessResponse(w, result)
}

func (r *Router) GetBotStats(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	id := vars["id"]

	bot, exists := r.botManager.GetBot(id)
	if !exists {
		NotFoundResponse(w, "bot not found")
		return
	}

	result := StatsResponse{
		UserCount:        bot.Stats.UserCount,
		GroupCount:       bot.Stats.GroupCount,
		TodayDAU:         bot.Stats.TodayDAU,
		QPS:              bot.Stats.QPS,
		SentMessages:     bot.Stats.SentMessages,
		ReceivedMessages: bot.Stats.ReceivedMessages,
	}
	SuccessResponse(w, result)
}