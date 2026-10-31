package survey

import (
	"context"

	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"

	"openmcdb/internal/ratelimit"
)

type Config struct {
	RateLimits  []ratelimit.Limit
	TrustRealIP bool
}

type snapshotStore interface {
	Insert(context.Context, Snapshot) error
	Count(context.Context) (int64, error)
}

type Module struct {
	store         snapshotStore
	submitLimiter *ratelimit.Limiter
	trustRealIP   bool
}

func New(db *sqlx.DB, cfg Config) *Module {
	return &Module{
		store:         &store{db: db},
		submitLimiter: ratelimit.New(cfg.RateLimits...),
		trustRealIP:   cfg.TrustRealIP,
	}
}

func (m *Module) RegisterRoutes(router chi.Router) {
	router.With(m.rateLimitSubmit).Post("/v1/submit", m.submitSnapshot)
}
