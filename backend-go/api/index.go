package handler

import (
	"net/http"

	"skill-progress-tracker/backend-go/app"
	"skill-progress-tracker/backend-go/internal/core"
)

// Handler is the Vercel Go Serverless Function entrypoint.
func Handler(w http.ResponseWriter, r *http.Request) {
	h, err := app.Handler()
	if err != nil {
		core.Logger.Error("Serverless initialization failed", "error", err)
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}
	h.ServeHTTP(w, r)
}
