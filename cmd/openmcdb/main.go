package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"openmcdb/internal/database"
	"openmcdb/internal/properties"
	"openmcdb/internal/ratelimit"
	"openmcdb/internal/server"
	"openmcdb/internal/survey"
)

func main() {
	log.Printf("openmcdb %s", properties.Version)
	if err := run(); err != nil {
		log.Fatalf("openmcdb: %v", err)
	}
}

func run() error {
	cfg := properties.Load()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	startupCtx, cancelStartup := context.WithTimeout(ctx, 15*time.Second)
	defer cancelStartup()
	db, err := database.Open(startupCtx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Printf("openmcdb: failed to close database: %v", err)
		}
	}()

	if err := survey.Init(startupCtx, db); err != nil {
		return err
	}
	cancelStartup()

	router := server.New(db.PingContext)
	surveyModule := survey.New(db, survey.Config{
		RateLimits: []ratelimit.Limit{
			{Max: cfg.SurveySubmitRateLimit4h, Window: 4 * time.Hour},
			{Max: cfg.SurveySubmitRateLimit24h, Window: 24 * time.Hour},
		},
		TrustRealIP: cfg.TrustRealIP,
	})
	if cfg.TrustRealIP {
		log.Printf("!! openmcdb: WARNING trusting X-Real-IP headers !!")
	}
	surveyModule.RegisterRoutes(router)

	httpServer := &http.Server{
		Addr:              cfg.Addr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	defer func() {
		if err := httpServer.Close(); err != nil {
			log.Printf("openmcdb: failed to close HTTP server: %v", err)
		}
	}()

	serverErr := make(chan error, 1)
	go func() {
		log.Printf("openmcdb: listening on http://%s", cfg.Addr)
		serverErr <- httpServer.ListenAndServe()
	}()
	select {
	case err := <-serverErr:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("HTTP server: %w", err)
		}
		return nil
	case <-ctx.Done():
		stop()
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("HTTP shutdown: %w", err)
	}
	return nil
}
