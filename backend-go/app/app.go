package app

import (
	"net/http"
	"sync"

	"skill-progress-tracker/backend-go/internal/config"
	"skill-progress-tracker/backend-go/internal/core"
	"skill-progress-tracker/backend-go/internal/database"
	"skill-progress-tracker/backend-go/internal/handlers"
)

var (
	initOnce   sync.Once
	appHandler http.Handler
	initErr    error
)

func Handler() (http.Handler, error) {
	initOnce.Do(func() {
		cfg := config.Load()
		if err := cfg.AssertProductionSecrets(); err != nil {
			initErr = err
			return
		}
		db, err := database.Open(cfg)
		if err != nil {
			initErr = err
			return
		}
		limiter := core.NewRateLimiter(cfg)
		appHandler = handlers.NewRouter(cfg, db, limiter)
	})
	return appHandler, initErr
}
