package server

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func New(pingDatabase func(context.Context) error) chi.Router {
	router := chi.NewRouter()
	router.Use(middleware.Recoverer)
	router.Get("/v1/health", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := pingDatabase(ctx); err != nil {
			WriteJSON(w, http.StatusServiceUnavailable, map[string]bool{"ok": false})
			return
		}
		WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})
	return router
}
