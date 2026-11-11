package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/credential"
	"danny.vn/trachuan/apps/server/internal/store"
	"danny.vn/trachuan/apps/server/internal/websession"
)

type routeSessionStore struct{ websession.Store }

func (routeSessionStore) GetSession(_ context.Context, hash string) (store.GetSessionRow, error) {
	now := time.Now()
	return store.GetSessionRow{ID: 1, AccountID: 1, TokenHash: hash, CreatedAt: now, LastSeenAt: now, ExpiresAt: now.Add(time.Hour)}, nil
}

func TestRoutes_CredentialsNeverCross(t *testing.T) {
	mux := http.NewServeMux()
	sessions := &websession.Manager{Store: routeSessionStore{}}
	login := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	protected := sessions.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	mountRoutes(mux, login, login, protected, protected, protected)
	mux.Handle("/mcp", credential.Middleware(&credential.Verifier{Store: drainStore{}, Audience: "http://localhost/mcp"}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })))
	for _, tt := range []struct {
		method, path string
		public       bool
	}{
		{"GET", "/api/v1/auth/google/start", true}, {"GET", "/api/v1/auth/google/callback", true},
		{"POST", "/api/v1/auth/logout", false}, {"GET", "/api/v1/me", false}, {"DELETE", "/api/v1/me", false},
		{"GET", "/api/v1/me/sessions", false}, {"DELETE", "/api/v1/me/sessions/1", false},
		{"GET", "/api/v1/me/tokens", false}, {"POST", "/api/v1/me/tokens", false}, {"DELETE", "/api/v1/me/tokens/1", false},
		{"POST", "/api/v1/search", false}, {"POST", "/api/v1/read", false}, {"GET", "/api/v1/documents/key", false}, {"GET", "/api/v1/status", false},
	} {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			r := httptest.NewRequest(tt.method, tt.path, nil)
			r.Header.Set("Authorization", "Bearer trc_"+strings.Repeat("A", 43))
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, r)
			want := http.StatusUnauthorized
			if tt.public {
				want = http.StatusNoContent
			}
			if w.Code != want {
				t.Fatalf("got %d, want %d", w.Code, want)
			}
		})
	}
	goodSession := httptest.NewRequest("GET", "/api/v1/status", nil)
	goodSession.AddCookie(&http.Cookie{Name: "__Host-session", Value: strings.Repeat("A", 43)})
	goodResponse := httptest.NewRecorder()
	mux.ServeHTTP(goodResponse, goodSession)
	if goodResponse.Code != http.StatusNoContent {
		t.Fatalf("valid session rejected: %d", goodResponse.Code)
	}
	goodBearer := httptest.NewRequest("POST", "/mcp", nil)
	goodBearer.Header.Set("Authorization", "Bearer trc_"+strings.Repeat("A", 43))
	goodResponse = httptest.NewRecorder()
	mux.ServeHTTP(goodResponse, goodBearer)
	if goodResponse.Code != http.StatusNoContent {
		t.Fatalf("valid bearer rejected: %d", goodResponse.Code)
	}
	r := httptest.NewRequest("POST", "/mcp", nil)
	r.AddCookie(&http.Cookie{Name: "__Host-session", Value: strings.Repeat("A", 43)})
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("session authorized MCP: %d", w.Code)
	}
}
