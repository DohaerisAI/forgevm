package httputil

import (
	"encoding/json"
	"net/http"
)

type ErrorCode string

const (
	CodeNotFound    ErrorCode = "NOT_FOUND"
	CodeBadRequest  ErrorCode = "BAD_REQUEST"
	CodeInternal    ErrorCode = "INTERNAL_ERROR"
	CodeUnauth      ErrorCode = "UNAUTHORIZED"
	CodeConflict    ErrorCode = "CONFLICT"
	CodeUnavailable ErrorCode = "UNAVAILABLE"
)

type APIError struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
}

func WriteJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func WriteError(w http.ResponseWriter, status int, code ErrorCode, msg string) {
	WriteJSON(w, status, APIError{Code: code, Message: msg})
}
