package survey

import (
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"

	"openmcdb/internal/server"
)

func (m *Module) rateLimitSubmit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		retryAfter, ok := m.submitLimiter.Allow(m.clientKey(r))
		if !ok {
			seconds := max(int(math.Ceil(retryAfter.Seconds())), 1)
			w.Header().Set("Retry-After", strconv.Itoa(seconds))
			server.WriteError(w, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (m *Module) clientKey(r *http.Request) string {
	if m.trustRealIP {
		if ip := net.ParseIP(strings.TrimSpace(r.Header.Get("X-Real-IP"))); ip != nil {
			return ip.String()
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
