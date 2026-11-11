package app

import "net/http"

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
