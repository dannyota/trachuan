package app

import (
	"net/http"
	"time"

	"danny.vn/trachuan/apps/server/internal/accountapi"
	"danny.vn/trachuan/apps/server/internal/apierror"
	"danny.vn/trachuan/apps/server/internal/credential"
	"danny.vn/trachuan/apps/server/internal/limits"
	"danny.vn/trachuan/apps/server/internal/mcpapi"
	"danny.vn/trachuan/apps/server/internal/oauth"
	"danny.vn/trachuan/apps/server/internal/passwordauth"
	"danny.vn/trachuan/apps/server/internal/reportsapi"
	"danny.vn/trachuan/apps/server/internal/webapi"
	"danny.vn/trachuan/apps/server/internal/weblogin"
	"danny.vn/trachuan/apps/server/internal/websession"
)

type webDeps struct {
	password     *passwordauth.Handler
	store        accountapi.Store
	reportStore  reportsapi.Store
	sessions     *websession.Manager
	login        *weblogin.Handler
	verifier     *credential.Verifier
	tools        mcpapi.Config
	loginLimit   *loginLimiter
	oauthDB      oauth.ClientDB
	oauthClients []oauth.Client
	publicURL    string
}

func newWebMux(d webDeps) *http.ServeMux {
	mux := http.NewServeMux()
	mountPasswordRoutes(mux, d)
	mountFactorRoutes(mux, d)
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
	reportHandler := d.sessions.Middleware(d.sessions.CSRF(reportsapi.New(reportsapi.Config{Store: d.reportStore, Reports: d.tools.Reports, Limiter: d.tools.Limiter})))
	mux.Handle("POST /api/v1/reports", reportHandler)
	mux.Handle("GET /api/v1/me/reports", reportHandler)
	mux.Handle("DELETE /api/v1/me/reports/{id}", reportHandler)
	corpus := webapi.New(webapi.Config{Tools: d.tools, Sessions: d.sessions})
	publicLimit := newLoginLimiter(d.loginLimit.clock, d.loginLimit.proxies)
	publicLimit.cost = time.Second
	mux.Handle("GET /api/v1/public/status", publicLimit.middleware(webapi.NewPublicStatus(d.tools)))
	taxonomy := publicLimit.middleware(webapi.NewPublicTaxonomy(d.tools))
	mux.Handle("GET /api/v1/public/taxonomy", taxonomy)
	mux.Handle("GET /api/v1/public/taxonomy/{code}", taxonomy)
	clients := oauth.NewClients(d.oauthDB, d.oauthClients, oauth.NewCIMDResolver())
	for pattern, handler := range map[string]http.Handler{
		"GET /.well-known/oauth-protected-resource":     oauth.ProtectedResourceMetadata(d.publicURL),
		"GET /.well-known/oauth-protected-resource/mcp": oauth.ProtectedResourceMetadata(d.publicURL),
		"GET /.well-known/oauth-authorization-server":   oauth.AuthorizationServerMetadata(d.publicURL),
		"GET /oauth/authorize":                          oauth.NewAuthorizeHandler(d.publicURL, d.oauthDB, clients.Resolve),
		"GET /oauth/consent/{handle}":                   oauth.NewConsentHandler(d.publicURL, d.oauthDB, d.sessions),
		"POST /oauth/consent/{handle}":                  oauth.NewConsentHandler(d.publicURL, d.oauthDB, d.sessions),
		"POST /oauth/token":                             oauth.NewTokenHandler(d.oauthDB, d.publicURL, d.verifier.EvictGrant),
		"POST /oauth/register":                          oauth.NewRegisterHandler(clients.Register),
		"POST /oauth/revoke":                            oauth.NewRevokeHandler(d.oauthDB, d.verifier.Evict, d.verifier.EvictGrant),
	} {
		switch pattern {
		case "POST /oauth/token", "POST /oauth/register", "POST /oauth/revoke":
			mux.Handle(pattern, d.loginLimit.oauthJSON(handler))
		case "GET /oauth/authorize", "GET /oauth/consent/{handle}", "POST /oauth/consent/{handle}":
			mux.Handle(pattern, d.loginLimit.oauthPage(handler))
		default:
			mux.Handle(pattern, d.loginLimit.middleware(handler))
		}
	}
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
	mux.Handle("GET /api/v1/me/grants", account)
	mux.Handle("DELETE /api/v1/me/grants/{id}", account)
	mux.Handle("GET /api/v1/me/tokens", account)
	mux.Handle("POST /api/v1/me/tokens", account)
	mux.Handle("DELETE /api/v1/me/tokens/{id}", account)
	mux.Handle("POST /api/v1/search", corpus)
	mux.Handle("POST /api/v1/read", corpus)
	mux.Handle("GET /api/v1/documents/{key}", corpus)
	mux.Handle("GET /api/v1/status", corpus)
}
