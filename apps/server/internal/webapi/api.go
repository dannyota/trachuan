package webapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"danny.vn/trachuan/apps/server/internal/apierror"
	"danny.vn/trachuan/apps/server/internal/limits"
	"danny.vn/trachuan/apps/server/internal/mcpapi"
	"danny.vn/trachuan/apps/server/internal/websession"
)

type Config struct {
	Tools    mcpapi.Config
	Sessions *websession.Manager
}

type api struct {
	cfg        Config
	operations *mcpapi.Operations
}

type endpoint func(*http.Request) (json.RawMessage, error)

func New(cfg Config) http.Handler {
	frameworks := newFrameworkRoutes(cfg)
	law := newLawRoutes(cfg)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/frameworks" || strings.HasPrefix(r.URL.Path, "/api/v1/frameworks/") {
			frameworks.ServeHTTP(w, r)
			return
		}
		law.ServeHTTP(w, r)
	})
}

func newLawRoutes(cfg Config) http.Handler {
	if cfg.Sessions == nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			apierror.Write(w, 503, "unavailable")
		})
	}
	a := &api{cfg: cfg, operations: mcpapi.NewOperations(cfg.Tools)}
	mux := http.NewServeMux()
	mux.Handle("POST /api/v1/search", a.route("search", limits.Search, a.search))
	mux.Handle("POST /api/v1/read", a.route("read", limits.Read, a.read))
	mux.Handle("GET /api/v1/documents/{key}", a.route("documents", limits.Read, a.documents))
	mux.Handle("GET /api/v1/status", a.route("status", limits.Read, a.status))
	return cfg.Sessions.Middleware(cfg.Sessions.CSRF(mux))
}

func (a *api) route(name string, class limits.Class, run endpoint) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		p, ok := websession.FromContext(r.Context())
		code := ""
		defer func() {
			if a.cfg.Tools.Logger != nil {
				a.cfg.Tools.Logger.Info("web request", "account_id", p.AccountID, "route", name, "code", code, "latency", time.Since(start))
			}
		}()
		fail := func(status int, c string) { code = c; apierror.Write(w, status, c) }
		if !ok || p.AccountID <= 0 {
			fail(401, "unauthenticated")
			return
		}
		if a.cfg.Tools.Limiter == nil {
			fail(503, "unavailable")
			return
		}
		if allowed, retry := a.cfg.Tools.Limiter.Allow(p.AccountID, class); !allowed {
			code = "rate_limited"
			apierror.RateLimited(w, retry)
			return
		}
		raw, err := run(r)
		if err != nil {
			status, c := httpError(err)
			if c == "rate_limited" {
				code = c
				apierror.RateLimited(w, 0)
				return
			}
			fail(status, c)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(raw)
	})
}
