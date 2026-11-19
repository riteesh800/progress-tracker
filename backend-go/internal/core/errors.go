package core

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type AppError struct {
	StatusCode int
	Code       string
	Message    string
}

func (e *AppError) Error() string {
	return fmt.Sprintf("%d %s: %s", e.StatusCode, e.Code, e.Message)
}

func NewAppError(statusCode int, code, message string) *AppError {
	return &AppError{
		StatusCode: statusCode,
		Code:       code,
		Message:    message,
	}
}

type ErrorDetail struct {
	Status  int    `json:"status"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ErrorPayload struct {
	Error ErrorDetail `json:"error"`
}

func BuildErrorPayload(statusCode int, code, message string) ErrorPayload {
	return ErrorPayload{
		Error: ErrorDetail{
			Status:  statusCode,
			Code:    code,
			Message: message,
		},
	}
}

func WriteJSON(w http.ResponseWriter, statusCode int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(data)
}

func WriteError(w http.ResponseWriter, statusCode int, code, message string) {
	WriteJSON(w, statusCode, BuildErrorPayload(statusCode, code, message))
}

func WriteAppError(w http.ResponseWriter, err error) {
	if appErr, ok := err.(*AppError); ok {
		WriteError(w, appErr.StatusCode, appErr.Code, appErr.Message)
		return
	}
	WriteError(w, http.StatusInternalServerError, "internal_error", "An unexpected error occurred.")
}

func WriteValidationError(w http.ResponseWriter) {
	WriteError(w, http.StatusUnprocessableEntity, "validation_error", "Invalid request payload.")
}
