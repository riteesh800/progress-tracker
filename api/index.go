package handler

import (
	"net/http"

	"skill-progress-tracker/backend-go/app"
)

// Handler is the Vercel Go Serverless Function entrypoint.
func Handler(w http.ResponseWriter, r *http.Request) {
	h, err := app.Handler()
	if err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"status":500,"code":"internal_error","message":"Something went wrong. Please try again."}}`))
		return
	}
	h.ServeHTTP(w, r)
}
