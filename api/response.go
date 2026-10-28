package api

import (
	"encoding/json"
	"net/http"
)

type Response struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

func JSONResponse(w http.ResponseWriter, code int, message string, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	resp := Response{
		Code:    code,
		Message: message,
		Data:    data,
	}

	if err := json.NewEncoder(w).Encode(resp); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func SuccessResponse(w http.ResponseWriter, data interface{}) {
	JSONResponse(w, 0, "success", data)
}

func ErrorResponse(w http.ResponseWriter, code int, message string) {
	JSONResponse(w, code, message, nil)
}

func BadRequestResponse(w http.ResponseWriter, message string) {
	ErrorResponse(w, 400, message)
}

func NotFoundResponse(w http.ResponseWriter, message string) {
	ErrorResponse(w, 404, message)
}

func InternalErrorResponse(w http.ResponseWriter, message string) {
	ErrorResponse(w, 500, message)
}