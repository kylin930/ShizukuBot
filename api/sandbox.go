package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"shizukubot/plugin"
)

type SandboxMessageRequest struct {
	Type     string `json:"type"`     // "c2c" or "group"
	Content  string `json:"content"`  // message content
	PluginID string `json:"plugin_id"` // 可选，指定只测试某个插件
}

type SandboxMessageResponse struct {
	Replies []string `json:"replies"`
}

func (r *Router) SandboxSend(w http.ResponseWriter, req *http.Request) {
	// 沙盒模式不依赖真实机器人，直接使用插件引擎处理消息
	botID := "sandbox"

	var msgReq SandboxMessageRequest
	if err := json.NewDecoder(req.Body).Decode(&msgReq); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}

	if msgReq.Content == "" {
		http.Error(w, `{"error":"content is required"}`, http.StatusBadRequest)
		return
	}

	msgType := msgReq.Type
	if msgType == "" {
		msgType = "c2c"
	}

	// 构造模拟的 EventContext
	eventType := "c2c_message"
	fakeUserID := "sandbox_user_001"
	fakeGroupID := ""
	fakeChannelID := ""

	if msgType == "group" {
		eventType = "group_at_message"
		fakeGroupID = "sandbox_group_001"
	}

	ctx := &plugin.EventContext{
		BotID:     botID,
		EventType: eventType,
		EventData: nil,
		Variables: make(map[string]interface{}),
		Message:   msgReq.Content,
		UserID:    fakeUserID,
		GroupID:   fakeGroupID,
		ChannelID: fakeChannelID,
		GuildID:   "",
		MessageID: fmt.Sprintf("sandbox_msg_%d", time.Now().UnixMilli()),
		LogDebug:  r.logManager.Debug,
	}

	replies, err := r.engine.ProcessEventAndGetRepliesWithPlugin(ctx, msgReq.PluginID)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	if replies == nil {
		replies = []string{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(SandboxMessageResponse{Replies: replies})
}

func (r *Router) SandboxPlugins(w http.ResponseWriter, req *http.Request) {
	// 沙盒模式返回所有插件（包括未启用的），方便选择测试
	plugins := r.pluginStore.List()
	if plugins == nil {
		plugins = []*plugin.Plugin{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(plugins)
}
