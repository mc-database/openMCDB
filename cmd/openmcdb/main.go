package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"openmcdb/internal/properties"
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
