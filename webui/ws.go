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
		return SameOrigin(r)
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
		u, ok := GetCurrentUser(r)
		if !ok || !u.IsAdmin {
			http.Error(w, "administrator required", 403)
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		conn.SetReadLimit(1024)
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		conn.SetPongHandler(func(string) error { return conn.SetReadDeadline(time.Now().Add(60 * time.Second)) })
		done := make(chan struct{})
		go func() {
			defer close(done)
			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					return
				}
			}
		}()

		defer func() { conn.Close(); <-done }()
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

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
			if err := func() error {
				conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				return conn.WriteMessage(websocket.TextMessage, data)
			}(); err != nil {
				return
			}
		}

		for {
			var entry core.LogEntry
			select {
			case <-done:
				return
			case <-r.Context().Done():
				return
			case <-ticker.C:
				u, ok := GetCurrentUser(r)
				if !ok || !u.IsAdmin {
					return
				}
				if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(10*time.Second)); err != nil {
					return
				}
				continue
			case e, ok := <-logChan:
				if !ok {
					return
				}
				entry = e
			}
			u, ok := GetCurrentUser(r)
			if !ok || !u.IsAdmin {
				return
			}
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
			if err := func() error {
				conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				return conn.WriteMessage(websocket.TextMessage, data)
			}(); err != nil {
				return
			}
		}
	}
}
