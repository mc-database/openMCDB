package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"openmcdb/internal/ratelimit"
	"openmcdb/internal/store"
)

type Config struct {
	DBPath           string
	SubmitRateLimits []ratelimit.Limit
	TrustRealIP      bool
}

type Server struct {
	router        chi.Router
	db            *store.DB
	submitLimiter *ratelimit.Limiter
	trustRealIP   bool
}

func New(cfg Config) (*Server, error) {
	db, err := store.Open(cfg.DBPath)
	if err != nil {
		return nil, err
	}

	s := &Server{
		db:            db,
		router:        chi.NewRouter(),
		submitLimiter: ratelimit.New(cfg.SubmitRateLimits...),
		trustRealIP:   cfg.TrustRealIP,
	}
	s.router.Use(middleware.Recoverer)
	s.router.Get("/", s.showStatus)
	s.router.Get("/v1/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})
	s.router.With(s.rateLimitSubmit).Post("/v1/submit", s.submitSnapshot)

	return s, nil
}

func (s *Server) Close() error {
	return s.db.Close()
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}
