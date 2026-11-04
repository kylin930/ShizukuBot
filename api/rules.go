package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"shizukubot/events"
	"shizukubot/rules"
	"unicode/utf8"

	"github.com/gorilla/mux"
)

func (r *Router) ruleBot(w http.ResponseWriter, req *http.Request) (string, bool) {
	id := mux.Vars(req)["id"]
	if !rules.ValidScopeID(id, 256) {
		BadRequestResponse(w, "invalid Bot ID")
		return "", false
	}
	if _, ok := r.botManager.GetBot(id); !ok {
		NotFoundResponse(w, "bot not found")
		return "", false
	}
	return id, true
}
func decodeRuleRequest(w http.ResponseWriter, req *http.Request, out any) bool {
	req.Body = http.MaxBytesReader(w, req.Body, rules.MaxRequestBytes)
	decoder := json.NewDecoder(req.Body)
	decoder.DisallowUnknownFields()
	err := decoder.Decode(out)
	if err == nil {
		if tail := decoder.Decode(new(any)); tail != io.EOF {
			err = errors.New("trailing JSON data")
		}
	}
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			http.Error(w, "rule request exceeds 1 MiB", http.StatusRequestEntityTooLarge)
		} else {
			BadRequestResponse(w, "invalid rule request: expected one JSON object with supported fields")
		}
		return false
	}
	return true
}

// GetRules returns a real Bot's immutable, versioned rule data.
func (r *Router) GetRules(w http.ResponseWriter, req *http.Request) {
	bot, ok := r.ruleBot(w, req)
	if !ok {
		return
	}
	set, err := r.rules.Snapshot(req.Context(), bot)
	if err != nil {
		InternalErrorResponse(w, "规则读取失败，原文件已保留；请检查规则数据及版本")
		return
	}
	SuccessResponse(w, set)
}

// ReplaceRules atomically applies an entire set with compare-and-swap revision.
func (r *Router) ReplaceRules(w http.ResponseWriter, req *http.Request) {
	bot, ok := r.ruleBot(w, req)
	if !ok {
		return
	}
	var body struct {
		Revision *uint64      `json:"revision"`
		Rules    []rules.Rule `json:"rules"`
	}
	if !decodeRuleRequest(w, req, &body) {
		return
	}
	if body.Revision == nil || body.Rules == nil {
		BadRequestResponse(w, "revision and rules array are required")
		return
	}
	// Only validation errors are returned verbatim. Filesystem errors stay internal.
	if _, err := rules.Prepare(body.Rules); err != nil {
		BadRequestResponse(w, err.Error())
		return
	}
	set, err := r.rules.Replace(req.Context(), bot, *body.Revision, body.Rules)
	if errors.Is(err, rules.ErrConflict) {
		JSONResponse(w, 409, "规则已被其他操作更新，请刷新后重新编辑", map[string]uint64{"revision": set.Revision})
		return
	}
	if err != nil {
		InternalErrorResponse(w, "规则保存失败，原规则仍然有效；请检查存储后重试")
		return
	}
	SuccessResponse(w, set)
}

// TestRules shares the production evaluator, supplying capture-only replies.
// It deliberately never publishes an event or invokes production Go plugins.
func (r *Router) TestRules(w http.ResponseWriter, req *http.Request) {
	bot, ok := r.ruleBot(w, req)
	if !ok {
		return
	}
	var body struct {
		Message string        `json:"message"`
		Source  events.Source `json:"source"`
		GroupID string        `json:"group_id"`
		UserID  string        `json:"user_id"`
		Rules   []rules.Rule  `json:"rules"`
	}
	if !decodeRuleRequest(w, req, &body) {
		return
	}
	if body.Message == "" || len(body.Message) > rules.MaxMessageBytes || !utf8.ValidString(body.Message) {
		BadRequestResponse(w, "message must contain 1–65536 UTF-8 bytes")
		return
	}
	if body.Source != events.Group && body.Source != events.C2C {
		BadRequestResponse(w, "source must be group or c2c")
		return
	}
	if body.Source == events.Group && !rules.ValidScopeID(body.GroupID, rules.MaxIDBytes) || body.Source == events.C2C && body.GroupID != "" {
		BadRequestResponse(w, "group source requires a valid group_id; c2c requires an empty group_id")
		return
	}
	if body.UserID == "" {
		body.UserID = "debug-user"
	}
	if !rules.ValidScopeID(body.UserID, rules.MaxIDBytes) {
		BadRequestResponse(w, "invalid user_id")
		return
	}
	if body.Rules != nil {
		if _, err := rules.Prepare(body.Rules); err != nil {
			BadRequestResponse(w, err.Error())
			return
		}
	}
	typ := events.Type("group_message")
	if body.Source == events.C2C {
		typ = "c2c_message"
	}
	out, err := r.rules.Preview(req.Context(), events.Event{BotID: bot, Type: typ, Source: body.Source, GroupID: body.GroupID, UserID: body.UserID, Message: body.Message}, body.Rules)
	if err != nil {
		InternalErrorResponse(w, "规则测试未完成，请检查规则存储或重试")
		return
	}
	SuccessResponse(w, out)
}
