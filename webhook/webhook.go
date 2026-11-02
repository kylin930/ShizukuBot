package webhook

import (
	"context"
	"encoding/json"
	"github.com/gorilla/mux"
	"github.com/tencent-connect/botgo/dto"
	"github.com/tencent-connect/botgo/interaction/signature"
	sdk "github.com/tencent-connect/botgo/interaction/webhook"
	"io"
	"net/http"
	"shizukubot/core"
	"shizukubot/transport/qq"
	"time"
)

type Handler struct{ botManager *core.BotManager }

func NewHandler(bm *core.BotManager) *Handler { return &Handler{bm} }

// ServeHTTP verifies the exact raw body and dispatches only to the URL's bot.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	bot, ok := h.botManager.GetBot(mux.Vars(r)["botID"])
	if !ok {
		http.Error(w, "bot not found", 404)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "invalid request", 400)
		return
	}
	pass, err := signature.Verify(bot.AppSecret, r.Header, body)
	if err != nil || !pass {
		bot.RecordTransportError("invalid_webhook_signature")
		http.Error(w, "invalid signature", 401)
		return
	}
	var p struct {
		Op   dto.OPCode      `json:"op"`
		Data json.RawMessage `json:"d"`
	}
	if json.Unmarshal(body, &p) != nil {
		http.Error(w, "invalid payload", 400)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	switch p.Op {
	case dto.HTTPCallbackValidation:
		var req dto.WHValidationReq
		if json.Unmarshal(p.Data, &req) != nil || req.PlainToken == "" || req.EventTs == "" {
			http.Error(w, "invalid validation", 400)
			return
		}
		ack := sdk.GenValidationACK(&req, r.Header, bot.AppSecret)
		if ack == nil {
			http.Error(w, "validation failed", 500)
			return
		}
		w.Write(ack)
	case dto.WSHeartbeat:
		var seq uint32
		if json.Unmarshal(p.Data, &seq) != nil {
			http.Error(w, "invalid heartbeat", 400)
			return
		}
		io.WriteString(w, sdk.GenHeartbeatACK(seq))
	case dto.WSDispatchEvent:
		if _, err := qq.Normalize(bot.ID, body); err != nil {
			http.Error(w, "invalid event", 400)
			return
		}
		if !bot.IsRunning() {
			bot.RecordTransportError("callback_while_stopped")
			io.WriteString(w, sdk.GenDispatchACK(false))
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		// d=0 acknowledges reception, not successful business replies. Admission
		// failures are retryable; handler failures must not cause message replays.
		accepted, err := bot.HandlePayloadAccepted(ctx, body)
		if err != nil {
			bot.RecordTransportError("callback_dispatch_failed")
		}
		io.WriteString(w, sdk.GenDispatchACK(accepted))
	default:
		http.Error(w, "unsupported opcode", 400)
	}
}
