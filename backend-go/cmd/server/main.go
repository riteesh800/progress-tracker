package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"skill-progress-tracker/backend-go/internal/config"
	"skill-progress-tracker/backend-go/internal/core"
	"skill-progress-tracker/backend-go/internal/database"
	"skill-progress-tracker/backend-go/internal/handlers"
)

func main() {
	cfg := config.Load()
	if err := cfg.AssertProductionSecrets(); err != nil {
		core.Logger.Error("Startup secret validation failed", "error", err)
		os.Exit(1)
	}

	db, err := database.Open(cfg)
	if err != nil {
		core.Logger.Error("Failed to open database", "error", err)
		os.Exit(1)
	}
	defer func() {
		_ = db.SQL.Close()
	}()

	limiter := core.NewRateLimiter(cfg)
	router := handlers.NewRouter(cfg, db, limiter)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8000"
	}

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		core.Logger.Info("Skill Progress Tracker Go API started", "port", port, "dialect", db.Dialect)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			core.Logger.Error("HTTP server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}
