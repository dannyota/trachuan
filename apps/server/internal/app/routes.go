package app

import (
	"net/http"

	"danny.vn/trachuan/apps/server/internal/accountapi"
	"danny.vn/trachuan/apps/server/internal/apierror"
	"danny.vn/trachuan/apps/server/internal/credential"
	"danny.vn/trachuan/apps/server/internal/limits"
	"danny.vn/trachuan/apps/server/internal/mcpapi"
	"danny.vn/trachuan/apps/server/internal/webapi"
	"danny.vn/trachuan/apps/server/internal/weblogin"
	"danny.vn/trachuan/apps/server/internal/websession"
)

type webDeps struct {
	store      accountapi.Store
	sessions   *websession.Manager
	login      *weblogin.Handler
	verifier   *credential.Verifier
	tools      mcpapi.Config
	loginLimit *loginLimiter
}

func newWebMux(d webDeps) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/mcp", credential.Middleware(d.verifier, mcpapi.New(d.tools)))
	account := d.sessions.Middleware(d.sessions.CSRF(accountapi.New(accountapi.Config{Store: d.store, Sessions: d.sessions, Verifier: d.verifier, Limiter: d.tools.Limiter, Clock: d.tools.Clock})))
	logout := d.sessions.Middleware(d.sessions.CSRF(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, _ := websession.FromContext(r.Context())
		if ok, wait := d.tools.Limiter.Allow(p.AccountID, limits.Read); !ok {
			apierror.RateLimited(w, wait)
			return
		}
		d.sessions.Logout(w, r)
	})))
	corpus := webapi.New(webapi.Config{Tools: d.tools, Sessions: d.sessions})
	mountRoutes(mux, d.loginLimit.middleware(d.login.Start), d.loginLimit.middleware(d.login.Callback), logout, account, corpus)
	return mux
}

func mountRoutes(mux *http.ServeMux, start, callback, logout, account, corpus http.Handler) {
	mux.Handle("GET /api/v1/auth/google/start", start)
	mux.Handle("GET /api/v1/auth/google/callback", callback)
	mux.Handle("POST /api/v1/auth/logout", logout)
	mux.Handle("GET /api/v1/me", account)
	mux.Handle("DELETE /api/v1/me", account)
	mux.Handle("GET /api/v1/me/sessions", account)
	mux.Handle("DELETE /api/v1/me/sessions/{id}", account)
	mux.Handle("GET /api/v1/me/tokens", account)
	mux.Handle("POST /api/v1/me/tokens", account)
	mux.Handle("DELETE /api/v1/me/tokens/{id}", account)
	mux.Handle("POST /api/v1/search", corpus)
	mux.Handle("POST /api/v1/read", corpus)
	mux.Handle("GET /api/v1/documents/{key}", corpus)
	mux.Handle("GET /api/v1/status", corpus)
}
