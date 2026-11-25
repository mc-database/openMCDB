package modrinth

import (
	"context"
	"sync"
	"time"
)

type pacer struct {
	mu       sync.Mutex
	interval time.Duration
	next     time.Time
	changed  chan struct{}
}

func newPacer(ratePerMinute int) *pacer {
	if ratePerMinute <= 0 {
		ratePerMinute = defaultRateLimitPerMinute
	}
	return &pacer{
		interval: max(time.Minute/time.Duration(ratePerMinute), time.Nanosecond),
		changed:  make(chan struct{}),
	}
}

func (p *pacer) Pause(d time.Duration) {
	until := time.Now().Add(d)
	p.mu.Lock()
	defer p.mu.Unlock()
	if until.After(p.next) {
		p.next = until
		close(p.changed)
		p.changed = make(chan struct{})
	}
}

func (p *pacer) Reserve(ctx context.Context) (time.Time, error) {
	for {
		p.mu.Lock()
		if err := ctx.Err(); err != nil {
			p.mu.Unlock()
			return time.Time{}, err
		}
		now := time.Now()
		wait := p.next.Sub(now)
		if wait <= 0 {
			p.next = now.Add(p.interval)
			p.mu.Unlock()
			return now, nil
		}
		changed := p.changed
		p.mu.Unlock()

		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return time.Time{}, ctx.Err()
		case <-changed:
			timer.Stop()
		case <-timer.C:
		}
	}
}
