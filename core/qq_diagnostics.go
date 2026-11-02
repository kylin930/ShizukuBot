package core

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/kylin930/botgo-plus/errs"
	"shizukubot/events"
)

// QQDiagnostics is a fixed-size, latest-only snapshot. No messages/targets/IDs/secrets.
// Success means OpenAPI accepted a send, not that a QQ client displayed it.
type QQDiagnostics struct {
	LastEventAt          *time.Time `json:"last_event_at,omitempty"`
	LastEventType        string     `json:"last_event_type,omitempty"`
	HasMessageID         bool       `json:"has_message_id"`
	LastReplyAt          *time.Time `json:"last_reply_at,omitempty"`
	LastSendFailureAt    *time.Time `json:"last_send_failure_at,omitempty"`
	LastSendFailure      string     `json:"last_send_failure,omitempty"`
	LastTransportError   string     `json:"last_transport_error,omitempty"`
	LastTransportErrorAt *time.Time `json:"last_transport_error_at,omitempty"`
	HTTPStatus           int        `json:"http_status,omitempty"`
	PlatformCode         int        `json:"platform_code,omitempty"`
	TraceID              string     `json:"trace_id,omitempty"`
}

func (b *Bot) Diagnostics() QQDiagnostics { b.mu.Lock(); defer b.mu.Unlock(); return b.diagnostics }

func safeQQError(err error) error {
	if err == nil {
		return nil
	}
	var p *errs.PlatformError
	if errors.As(err, &p) {
		return p
	}
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	var n net.Error
	if errors.As(err, &n) && n.Timeout() {
		return context.DeadlineExceeded
	}
	// Third-party errors may embed URLs, bodies or credentials.
	return errors.New("QQ network or SDK operation failed")
}
func sendCategory(err error) string {
	if err == nil {
		return ""
	}
	switch {
	case errors.Is(err, ErrMissingReplyID):
		return "missing_reply_id"
	case errors.Is(err, ErrReplyLimit):
		return "reply_limit"
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout_unknown_delivery"
	}
	var p *errs.PlatformError
	if errors.As(err, &p) {
		switch {
		case p.Status == 401 || p.Code == 11244:
			return "authentication"
		case p.Status == 429 || p.Code == 22009 || p.Code == 40034100:
			return "platform_limit"
		case p.Code == 304027 || p.Code == 304103 || p.Code == 40034005 || p.Code == 40034026:
			return "passive_reply_expired"
		case p.Code == 40034128:
			return "passive_reply_time_or_count_limit"
		case p.Code == 40034101 || p.Code == 40054003:
			return "not_group_member"
		case p.Code == 40034105:
			return "active_send_not_permitted"
		case p.Code == 11253 || p.Code == 304036 || p.Code == 304037 || p.Code == 40034127:
			return "target_or_feature_permission"
		case p.Status == 403:
			return "permission_or_ip_policy" // do not guess which
		default:
			return "unknown_platform_error"
		}
	}
	return "network_or_protocol"
}
func (b *Bot) recordEvent(e events.Event) {
	now := time.Now().UTC()
	b.mu.Lock()
	b.diagnostics.LastEventAt = &now
	b.diagnostics.LastEventType = e.PlatformType
	b.diagnostics.HasMessageID = e.MessageID != ""
	b.mu.Unlock()
	b.logDebug("QQ event mode=%s type=%s has_message_id=%t", b.Mode, e.PlatformType, e.MessageID != "")
}
func (b *Bot) RecordTransportError(category string) {
	// Callers pass fixed categories only, never raw error strings.
	now := time.Now().UTC()
	b.mu.Lock()
	b.diagnostics.LastTransportError = category
	b.diagnostics.LastTransportErrorAt = &now
	b.mu.Unlock()
	b.logWarn("QQ transport mode=%s category=%s", b.Mode, category)
}
func (b *Bot) recordSend(e events.Event, kind string, err error) {
	b.mu.Lock()
	if err == nil {
		now := time.Now().UTC()
		if e.MessageID != "" || e.EventID != "" {
			b.diagnostics.LastReplyAt = &now
		}
	} else {
		now := time.Now().UTC()
		b.diagnostics.LastSendFailureAt = &now
		b.diagnostics.HTTPStatus, b.diagnostics.PlatformCode, b.diagnostics.TraceID = 0, 0, ""
		b.diagnostics.LastSendFailure = sendCategory(err)
		var p *errs.PlatformError
		if errors.As(err, &p) {
			b.diagnostics.HTTPStatus = p.Status
			b.diagnostics.PlatformCode = p.Code
			b.diagnostics.TraceID = p.TraceID
		}
	}
	b.mu.Unlock()
	if err == nil {
		b.Stats.IncSent()
		b.logInfo("QQ send mode=%s source=%s kind=%s success=true", b.Mode, e.Source, kind)
	} else {
		b.logError("QQ send mode=%s source=%s kind=%s category=%s error=%s", b.Mode, e.Source, kind, sendCategory(err), fmt.Sprint(err))
	}
}
