package core

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"skill-progress-tracker/backend-go/internal/config"
)

type RateLimiter struct {
	mu      sync.Mutex
	hits    map[string][]time.Time
	enabled bool
}

func NewRateLimiter(cfg *config.Settings) *RateLimiter {
	return &RateLimiter{
		hits:    make(map[string][]time.Time),
		enabled: !cfg.IsTest(),
	}
}

func (rl *RateLimiter) SetEnabled(enabled bool) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.enabled = enabled
}

func (rl *RateLimiter) Reset() {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.hits = make(map[string][]time.Time)
}

func GetClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		first := strings.TrimSpace(strings.Split(xff, ",")[0])
		if first != "" {
			return first
		}
	}
	if xri := strings.TrimSpace(r.Header.Get("X-Real-IP")); xri != "" {
		return xri
	}
	if r.RemoteAddr != "" {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err == nil && host != "" {
			return host
		}
		return r.RemoteAddr
	}
	return "127.0.0.1"
}

func (rl *RateLimiter) EnforceLimit(cfg *config.Settings, r *http.Request, key string, maxHits int, windowSeconds int) error {
	ident := key + ":" + GetClientIP(r)
	return rl.EnforceLimitKey(cfg, ident, maxHits, windowSeconds, "Too many requests. Please wait and try again.")
}

func (rl *RateLimiter) EnforceLimitKey(_ *config.Settings, key string, maxHits int, windowSeconds int, errMsg string) error {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	if !rl.enabled {
		return nil
	}

	now := time.Now()
	window := time.Duration(windowSeconds) * time.Second

	if len(rl.hits) > 5000 {
		for k, q := range rl.hits {
			if len(q) == 0 || now.Sub(q[len(q)-1]) > window {
				delete(rl.hits, k)
			}
		}
	}

	q := rl.hits[key]
	validIdx := 0
	for validIdx < len(q) && now.Sub(q[validIdx]) > window {
		validIdx++
	}
	q = q[validIdx:]

	if len(q) >= maxHits {
		rl.hits[key] = q
		if errMsg == "" {
			errMsg = "Too many requests. Please wait and try again."
		}
		return NewAppError(http.StatusTooManyRequests, "rate_limited", errMsg)
	}

	q = append(q, now)
	rl.hits[key] = q
	return nil
}
