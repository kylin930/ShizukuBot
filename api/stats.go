package api

import (
	"github.com/gorilla/mux"
	"net/http"
	stats "shizukubot/storage/stats"
	"strconv"
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
	if store := r.botManager.StatsStore(); store != nil {
		snapshot, err := store.Snapshot(req.Context(), r.botManager.BotIDs())
		if err != nil {
			InternalErrorResponse(w, "statistics read failed: "+err.Error())
			return
		}
		SuccessResponse(w, map[string]interface{}{"user_count": snapshot.UserCount, "group_count": snapshot.GroupCount, "today_dau": snapshot.TodayDAU, "qps": r.botManager.QPS(), "sent_messages": snapshot.SentMessages, "received_messages": snapshot.ReceivedMessages, "hourly_stats": compatibleHours(snapshot.Hourly), "retention_days": store.Settings().RetentionDays, "timezone": store.Settings().Timezone})
		return
	}
	stats := r.botManager.GetTotalStats()
	SuccessResponse(w, map[string]interface{}{"user_count": stats.UserCount, "group_count": stats.GroupCount, "today_dau": stats.TodayDAU, "qps": stats.QPS, "sent_messages": stats.SentMessages, "received_messages": stats.ReceivedMessages, "hourly_stats": r.botManager.GetTodayHourlyStats()})
}
func (r *Router) GetBotStats(w http.ResponseWriter, req *http.Request) {
	id := mux.Vars(req)["id"]
	bot, ok := r.botManager.GetBot(id)
	if !ok {
		NotFoundResponse(w, "bot not found")
		return
	}
	if store := r.botManager.StatsStore(); store != nil {
		v, err := store.Snapshot(req.Context(), []string{id})
		if err != nil {
			InternalErrorResponse(w, "statistics read failed")
			return
		}
		SuccessResponse(w, map[string]interface{}{"user_count": v.UserCount, "group_count": v.GroupCount, "today_dau": v.TodayDAU, "qps": bot.Stats.RealtimeQPS(), "sent_messages": v.SentMessages, "received_messages": v.ReceivedMessages, "hourly_stats": compatibleHours(v.Hourly)})
		return
	}
	v := bot.Stats.Snapshot()
	SuccessResponse(w, StatsResponse{v.UserCount, v.GroupCount, v.TodayDAU, v.QPS, v.SentMessages, v.ReceivedMessages})
}
func (r *Router) StatsHistory(w http.ResponseWriter, req *http.Request) {
	days := 30
	if raw := req.URL.Query().Get("days"); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil {
			BadRequestResponse(w, "days must be 7, 30 or 90")
			return
		}
		days = v
	}
	if days != 7 && days != 30 && days != 90 {
		BadRequestResponse(w, "days must be 7, 30 or 90")
		return
	}
	ids := r.botManager.BotIDs()
	if id := req.URL.Query().Get("bot_id"); id != "" {
		if _, ok := r.botManager.GetBot(id); !ok {
			NotFoundResponse(w, "bot not found")
			return
		}
		ids = []string{id}
	}
	store := r.botManager.StatsStore()
	if store == nil {
		SuccessResponse(w, []interface{}{})
		return
	}
	history, err := store.History(req.Context(), ids, days)
	if err != nil {
		InternalErrorResponse(w, "statistics history read failed")
		return
	}
	SuccessResponse(w, history)
}
func (r *Router) StatsStatus(w http.ResponseWriter, req *http.Request) {
	store := r.botManager.StatsStore()
	if store == nil {
		SuccessResponse(w, map[string]string{"state": "memory"})
		return
	}
	SuccessResponse(w, store.Status())
}
func (r *Router) StatsCleanup(w http.ResponseWriter, req *http.Request) {
	store := r.botManager.StatsStore()
	if store == nil {
		BadRequestResponse(w, "statistics storage unavailable")
		return
	}
	if err := store.Cleanup(req.Context()); err != nil {
		InternalErrorResponse(w, "statistics cleanup failed: "+err.Error())
		return
	}
	SuccessResponse(w, store.Status())
}

// Existing clients used exported Go field names; keep those alongside lowercase
// fields consumed by the WebUI and the new history API conventions.
func compatibleHours(hours []stats.Hour) []map[string]interface{} {
	result := make([]map[string]interface{}, len(hours))
	for i, h := range hours {
		result[i] = map[string]interface{}{"Hour": h.Hour, "Sent": h.Sent, "Received": h.Received, "hour": h.Hour, "sent": h.Sent, "received": h.Received}
	}
	return result
}
