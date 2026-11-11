// Package reportsapi serves report routes for web sessions.
package reportsapi

import (
	"context"
	"net/http"
	"time"

	"danny.vn/trachuan/apps/server/internal/apierror"
	"danny.vn/trachuan/apps/server/internal/limits"
	"danny.vn/trachuan/apps/server/internal/reports"
	"danny.vn/trachuan/apps/server/internal/store"
	"danny.vn/trachuan/apps/server/internal/websession"
)

// Store supplies account-scoped report queries.
type Store interface {
	ListOwnReports(context.Context, store.ListOwnReportsParams) ([]store.IssueReport, error)
	WithdrawOwnReport(context.Context, store.WithdrawOwnReportParams) (int64, error)
}

// Submitter admits reports through the shared report service.
type Submitter interface {
	Submit(context.Context, int64, reports.Input) (reports.Result, error)
}

// Limiter consumes the shared account request budget.
type Limiter interface {
	Allow(int64, limits.Class) (bool, time.Duration)
}

// Config supplies report dependencies.
type Config struct {
	Store   Store
	Reports Submitter
	Limiter Limiter
}
type handler struct {
	cfg Config
	mux *http.ServeMux
}

// New creates the report handler. Install session and CSRF middleware outside it.
func New(cfg Config) http.Handler {
	h := &handler{cfg: cfg, mux: http.NewServeMux()}
	h.mux.HandleFunc("POST /api/v1/reports", h.submit)
	h.mux.HandleFunc("GET /api/v1/me/reports", h.list)
	h.mux.HandleFunc("DELETE /api/v1/me/reports/{id}", h.withdraw)
	return h
}
func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p, ok := websession.FromContext(r.Context())
	if !ok {
		apierror.Write(w, 401, "unauthenticated")
		return
	}
	if h.cfg.Limiter == nil {
		apierror.Write(w, 503, "unavailable")
		return
	}
	if ok, wait := h.cfg.Limiter.Allow(p.AccountID, limits.Read); !ok {
		apierror.RateLimited(w, wait)
		return
	}
	h.mux.ServeHTTP(w, r)
}
