package webui

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"shizukubot/core"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type LogMessage struct {
	Timestamp string `json:"timestamp"`
	BotID     string `json:"bot_id"`
	Level     string `json:"level"`
	Message   string `json:"message"`
}

func LogsWSHandler(logManager *core.LogManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		logChan := logManager.Subscribe(100)
		defer logManager.Unsubscribe(logChan)

		history := logManager.GetHistory("", 50)
		for _, entry := range history {
			msg := LogMessage{
				Timestamp: entry.Timestamp.Format(time.RFC3339),
				BotID:     entry.BotID,
				Level:     entry.Level.String(),
				Message:   entry.Message,
			}
			data, _ := json.Marshal(msg)
			conn.WriteMessage(websocket.TextMessage, data)
		}

		for entry := range logChan {
			msg := LogMessage{
				Timestamp: entry.Timestamp.Format(time.RFC3339),
				BotID:     entry.BotID,
				Level:     entry.Level.String(),
				Message:   entry.Message,
			}
			data, err := json.Marshal(msg)
			if err != nil {
				continue
			}
			if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
				return
			}
		}
	}
}