package modrinth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

var ErrPermanent = errors.New("modrinth: permanent error")

type client struct {
	base       string
	ua         string
	http       *http.Client
	pacer      *pacer
	scope      string
	mu         sync.Mutex
	pathCounts map[string]int64
	attempts   int64
	rateLimits int64
}

func newClient(cfg CrawlConfig) *client {
	return &client{
		base:       strings.TrimRight(cfg.APIBase, "/"),
		ua:         cfg.UserAgent,
		http:       &http.Client{Timeout: 120 * time.Second},
		pacer:      newPacer(cfg.RateLimitPerMinute),
		scope:      cfg.ScopeFacet,
		pathCounts: make(map[string]int64),
	}
}

func (c *client) RequestStats() (paths map[string]int64, totalAttempts int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make(map[string]int64, len(c.pathCounts))
	maps.Copy(out, c.pathCounts)
	return out, c.attempts
}

func (c *client) RateLimitCount() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.rateLimits
}

func (c *client) getJSON(ctx context.Context, path string, query url.Values, out any) error {
	values := query
	if values == nil {
		values = url.Values{}
	}
	endpoint := c.base + path
	if len(values) > 0 {
		endpoint += "?" + values.Encode()
	}
	if err := c.getJSONEndpoint(ctx, endpoint, out); err != nil {
		return err
	}
	c.mu.Lock()
	c.pathCounts[path]++
	c.mu.Unlock()
	return nil
}

func (c *client) getJSONEndpoint(ctx context.Context, endpoint string, out any) error {
	var lastErr error
	for attempt := range 12 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err := c.wait(ctx); err != nil {
			return err
		}
		c.mu.Lock()
		c.attempts++
		c.mu.Unlock()

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", c.ua)

		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("modrinth: request %s: %w", endpoint, err)
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			if err := c.sleepBackoff(ctx, attempt, 0); err != nil {
				return err
			}
			continue
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			c.mu.Lock()
			c.rateLimits++
			c.mu.Unlock()
			c.pacer.Pause(rateLimitRetryAfter(resp, nil, 0))
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 200<<20))
		_ = resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("modrinth: read %s: %w", endpoint, err)
			if resp.StatusCode == http.StatusTooManyRequests {
				c.pacer.Pause(rateLimitRetryAfter(resp, nil, 5*time.Second))
			}
			continue
		}

		switch {
		case resp.StatusCode == http.StatusOK:
			if err := json.Unmarshal(body, out); err != nil {
				return fmt.Errorf("modrinth: decode %s: %w", endpoint, err)
			}
			return nil
		case resp.StatusCode == http.StatusTooManyRequests:
			wait := rateLimitRetryAfter(resp, body, 5*time.Second)
			lastErr = fmt.Errorf("modrinth: rate limited on %s (retry in %s)", endpoint, wait)
			c.pacer.Pause(wait)
			continue
		case resp.StatusCode == http.StatusNotFound:
			return fmt.Errorf("%w: 404 on %s", ErrPermanent, endpoint)
		case resp.StatusCode >= 500:
			lastErr = fmt.Errorf("modrinth: %s returned %s: %s", endpoint, resp.Status, truncate(string(body), 200))
			if err := c.sleepBackoff(ctx, attempt, 0); err != nil {
				return err
			}
			continue
		default:
			return fmt.Errorf("%w: %s returned %s: %s", ErrPermanent, endpoint, resp.Status, truncate(string(body), 200))
		}
	}
	return fmt.Errorf("modrinth: giving up on %s after retries: %w", endpoint, lastErr)
}

func (c *client) wait(ctx context.Context) error {
	_, err := c.pacer.Reserve(ctx)
	return err
}

func (c *client) sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (c *client) sleepBackoff(ctx context.Context, attempt int, serverHint time.Duration) error {
	base := 2 * time.Second
	backoff := base
	for range attempt {
		backoff *= 2
	}
	if backoff > 60*time.Second {
		backoff = 60 * time.Second
	}
	if serverHint > backoff {
		backoff = serverHint
	}
	return c.sleep(ctx, backoff)
}

func rateLimitRetryAfter(resp *http.Response, body []byte, fallback time.Duration) time.Duration {
	if header := strings.TrimSpace(resp.Header.Get("Retry-After")); header != "" {
		if wait, ok := retryDelay(header, time.Second); ok {
			return wait
		}
		if when, err := http.ParseTime(header); err == nil {
			return max(time.Until(when), 0)
		}
	}
	const marker = "retry after "
	if i := strings.Index(strings.ToLower(string(body)), marker); i >= 0 {
		rest := strings.TrimSpace(string(body)[i+len(marker):])
		fields := strings.Fields(rest)
		if len(fields) >= 2 && strings.EqualFold(strings.TrimRight(fields[1], `",.;}]`), "milliseconds") {
			if wait, ok := retryDelay(fields[0], time.Millisecond); ok {
				return wait
			}
		}
	}
	if wait, ok := retryDelay(strings.TrimSpace(resp.Header.Get("X-RateLimit-Reset")), time.Second); ok {
		return wait
	}
	return fallback
}

func retryDelay(raw string, unit time.Duration) (time.Duration, bool) {
	n, err := strconv.ParseUint(raw, 10, 64)
	const maxDuration = time.Duration(1<<63 - 1)
	if err != nil || n > uint64(maxDuration/unit) {
		return 0, false
	}
	return time.Duration(n) * unit, true
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
