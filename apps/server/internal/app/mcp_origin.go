package app

import (
	"log/slog"
	"net/http"
	"time"
)

type mcpOriginPolicy struct {
	allowed map[string]struct{}
	mode    string
	tracker mcpOriginTracker
}

func newMCPOriginPolicy(public string, origins []string, mode string, logger *slog.Logger, clock func() time.Time) *mcpOriginPolicy {
	if mode == "" {
		mode = "enforce"
	}
	if logger == nil {
		logger = slog.Default()
	}
	if clock == nil {
		clock = time.Now
	}
	p := &mcpOriginPolicy{allowed: make(map[string]struct{}), mode: mode, tracker: mcpOriginTracker{clock: clock, logger: logger}}
	if public != "" {
		p.allowed[public] = struct{}{}
	}
	for _, origin := range origins {
		p.allowed[origin] = struct{}{}
	}
	return p
}

func (p *mcpOriginPolicy) middleware(method string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if method != "" && r.Method != method {
			next.ServeHTTP(w, r)
			return
		}
		values := r.Header.Values("Origin")
		outside := len(values) > 1
		if len(values) == 1 {
			_, allowed := p.allowed[values[0]]
			outside = !allowed
		}
		p.tracker.observe(r.Context(), outside, values, r.URL.Path, p.mode)
		if outside && p.mode == "enforce" {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "no-store")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":"invalid_request"}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}
