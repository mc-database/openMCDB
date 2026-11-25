package modrinth

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/jmoiron/sqlx"
)

type Module struct {
	db  *sqlx.DB
	cfg CrawlConfig
}

func Init(ctx context.Context, db *sqlx.DB, cfg CrawlConfig) (*Module, error) {
	cfg, err := NormalizeCrawlConfig(cfg)
	if err != nil {
		return nil, err
	}
	if cfg.Enabled {
		if err := Migrate(ctx, db); err != nil {
			return nil, err
		}
	}
	return &Module{db: db, cfg: cfg}, nil
}

func (m *Module) Start(ctx context.Context) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		m.run(ctx)
	}()
	return done
}

func (m *Module) run(ctx context.Context) {
	if !m.cfg.Enabled {
		log.Printf("openmcdb/modrinth: crawler disabled")
		return
	}
	if ctx.Err() != nil {
		return
	}
	if m.cfg.RunOnStart {
		m.crawlOnce(ctx, "startup")
	}
	next := nextRunTime(time.Now().UTC(), m.cfg.HourUTC, m.cfg.MinuteUTC)
	runSchedule(ctx, next, m.cfg.Interval, m.crawlOnce)
}

func runSchedule(ctx context.Context, next time.Time, interval time.Duration, crawl func(context.Context, string)) {
	for {
		if d := time.Until(next); d > 0 {
			log.Printf("openmcdb/modrinth: next crawl at %s (%s from now)", next.Format(time.RFC3339), d.Round(time.Second))
			timer := time.NewTimer(d)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
		if ctx.Err() != nil {
			return
		}
		crawl(ctx, "scheduled")
		next = advanceRunTime(time.Now(), next, interval)
	}
}

func advanceRunTime(now, previous time.Time, interval time.Duration) time.Time {
	next := previous.Add(interval)
	if !next.After(now) {
		next = now.Add(interval - now.Sub(next)%interval)
	}
	return next
}

func (m *Module) crawlOnce(ctx context.Context, reason string) {
	crawlCtx, cancel := context.WithTimeout(ctx, m.cfg.Timeout)
	defer cancel()
	log.Printf("openmcdb/modrinth: crawl starting (%s)", reason)
	start := time.Now()
	if err := RunOnce(crawlCtx, m.db, m.cfg); err != nil {
		if errors.Is(err, ErrCrawlBusy) {
			log.Printf("openmcdb/modrinth: crawl skipped (%s): another crawl is running", reason)
			return
		}
		if ctx.Err() != nil {
			log.Printf("openmcdb/modrinth: crawl aborted: %v", err)
			return
		}
		log.Printf("openmcdb/modrinth: crawl failed after %s: %v", time.Since(start).Round(time.Second), err)
		return
	}
	log.Printf("openmcdb/modrinth: crawl finished in %s", time.Since(start).Round(time.Second))
}

func nextRunTime(now time.Time, hour, minute int) time.Time {
	now = now.UTC()
	t := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, time.UTC)
	if !t.After(now) {
		t = t.Add(24 * time.Hour)
	}
	return t
}
