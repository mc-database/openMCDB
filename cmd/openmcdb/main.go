package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"openmcdb/internal/properties"
	"openmcdb/internal/ratelimit"
	"os"
	"os/signal"
	"syscall"
	"time"

	"openmcdb/internal/server"
)

func main() {
	log.Printf("openmcdb %s", properties.Version)

	cfg := server.Config{
		DBPath: properties.DBPath,
		SubmitRateLimits: []ratelimit.Limit{
			{Max: properties.SubmitRateLimit4h, Window: 4 * time.Hour},
			{Max: properties.SubmitRateLimit24h, Window: 24 * time.Hour},
		},
		TrustRealIP: properties.TrustRealIP,
	}
	if cfg.TrustRealIP {
		log.Printf("!! openmcdb: WARNING trusting X-Real-IP headers !!")
	}

	srv, err := server.New(cfg)
	if err != nil {
		log.Fatalf("openmcdb: %v", err)
	}

	httpServer := &http.Server{
		Addr:              properties.Addr,
		Handler:           srv,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	serverErr := make(chan error, 1)
	go func() {
		//goland:noinspection HttpUrlsUsage
		log.Printf("openmcdb: listening on http://%s", properties.Addr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()
	select {
	case err := <-serverErr:
		_ = srv.Close()
		log.Fatalf("openmcdb: %v", err)
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("openmcdb: shutdown failed: %v", err)
	}
	if err := srv.Close(); err != nil {
		log.Printf("openmcdb: failed to close server: %v", err)
	}
}
