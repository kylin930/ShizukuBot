package api

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
)

func (r *Router) SendGroupMessage(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	id := vars["id"]

	bot, exists := r.botManager.GetBot(id)
	if !exists {
		NotFoundResponse(w, "bot not found")
		return
	}

	var reqBody struct {
		GroupID string `json:"group_id"`
		Content string `json:"content"`
	}

	if err := json.NewDecoder(req.Body).Decode(&reqBody); err != nil {
		BadRequestResponse(w, "invalid request body")
		return
	}

	if reqBody.GroupID == "" || reqBody.Content == "" {
		BadRequestResponse(w, "group_id and content are required")
		return
	}

	if err := bot.SendGroupMessage(reqBody.GroupID, reqBody.Content); err != nil {
		InternalErrorResponse(w, err.Error())
		return
	}

	SuccessResponse(w, nil)
}

func (r *Router) SendC2CMessage(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	id := vars["id"]

	bot, exists := r.botManager.GetBot(id)
	if !exists {
		NotFoundResponse(w, "bot not found")
		return
	}

	var reqBody struct {
		UserID  string `json:"user_id"`
		Content string `json:"content"`
	}

	if err := json.NewDecoder(req.Body).Decode(&reqBody); err != nil {
		BadRequestResponse(w, "invalid request body")
		return
	}

	if reqBody.UserID == "" || reqBody.Content == "" {
		BadRequestResponse(w, "user_id and content are required")
		return
	}

	if err := bot.SendC2CMessage(reqBody.UserID, reqBody.Content); err != nil {
		InternalErrorResponse(w, err.Error())
		return
	}

	SuccessResponse(w, nil)
}