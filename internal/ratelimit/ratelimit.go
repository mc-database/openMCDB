package ratelimit

import (
	"sync"
	"time"
)

const sweepInterval = time.Minute

type Limit struct {
	Max    int
	Window time.Duration
}

type Limiter struct {
	mu        sync.Mutex
	limits    []Limit
	hits      map[string][]time.Time
	lastSeen  map[string]time.Time
	maxWindow time.Duration
	lastSweep time.Time
	now       func() time.Time
}

func New(limits ...Limit) *Limiter {
	active := make([]Limit, 0, len(limits))
	var maxWindow time.Duration
	for _, l := range limits {
		if l.Max <= 0 || l.Window <= 0 {
			continue
		}
		active = append(active, l)
		if l.Window > maxWindow {
			maxWindow = l.Window
		}
	}
	now := time.Now()
	return &Limiter{
		limits:    active,
		hits:      make(map[string][]time.Time),
		lastSeen:  make(map[string]time.Time),
		maxWindow: maxWindow,
		lastSweep: now,
		now:       time.Now,
	}
}

func (l *Limiter) Allow(key string) (time.Duration, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()

	if len(l.limits) == 0 {
		return 0, true
	}

	l.sweep(now)

	hits := l.prune(l.hits[key], now.Add(-l.maxWindow))

	var retryAfter time.Duration
	for _, limit := range l.limits {
		cutoff := now.Add(-limit.Window)
		count := 0
		var oldest time.Time
		for _, t := range hits {
			if t.After(cutoff) {
				if count == 0 {
					oldest = t
				}
				count++
			}
		}
		if count >= limit.Max {
			if wait := oldest.Add(limit.Window).Sub(now); wait > retryAfter {
				retryAfter = wait
			}
		}
	}

	if retryAfter > 0 {
		l.hits[key] = hits
		return retryAfter, false
	}

	l.hits[key] = append(hits, now)
	l.lastSeen[key] = now
	return 0, true
}

func (l *Limiter) prune(hits []time.Time, cutoff time.Time) []time.Time {
	i := 0
	for i < len(hits) && !hits[i].After(cutoff) {
		i++
	}
	return hits[i:]
}

func (l *Limiter) sweep(now time.Time) {
	if now.Sub(l.lastSweep) < sweepInterval {
		return
	}
	l.lastSweep = now
	cutoff := now.Add(-l.maxWindow)
	for key, seen := range l.lastSeen {
		if seen.Before(cutoff) {
			delete(l.hits, key)
			delete(l.lastSeen, key)
		}
	}
}
