package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/credential"
	"danny.vn/trachuan/apps/server/internal/limits"
	"danny.vn/trachuan/apps/server/internal/mcpapi"
	"danny.vn/trachuan/apps/server/internal/store"
	"danny.vn/trachuan/apps/server/internal/weblogin"
	"danny.vn/trachuan/apps/server/internal/websession"
)

type mcpCacheCredentialStore struct {
	row     store.GetTokenRow
	err     error
	lookups atomic.Int32
}

func (*mcpCacheCredentialStore) GetCredential(context.Context, string) (store.GetCredentialRow, error) {
	return store.GetCredentialRow{}, credential.ErrUnauthorized
}

func (*mcpCacheCredentialStore) TouchPersonalToken(context.Context, string) error { return nil }

func (s *mcpCacheCredentialStore) GetToken(context.Context, string) (store.GetTokenRow, error) {
	s.lookups.Add(1)
	return s.row, s.err
}

func (*mcpCacheCredentialStore) TouchOAuthGrant(context.Context, int64) error { return nil }

func TestMCPRoute_AllAuthenticationOutcomesAreNoStore(t *testing.T) {
	for _, tc := range []struct {
		name, scope string
		auth        bool
		storeErr    error
		status      int
	}{
		{name: "unauthenticated", status: http.StatusUnauthorized},
		{name: "scope rejected", auth: true, scope: "other", status: http.StatusForbidden},
		{name: "store unavailable", auth: true, scope: "mcp:read", storeErr: errors.New("invented credential store failure"), status: http.StatusServiceUnavailable},
		{name: "success", auth: true, scope: "mcp:read", status: http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const publicURL = "https://example.test"
			now := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
			clock := func() time.Time { return now }
			db := &mcpCacheCredentialStore{row: store.GetTokenRow{Kind: "access", AccountID: 7, GrantID: 9, Resource: publicURL + "/mcp", Scope: tc.scope, ExpiresAt: now.Add(time.Hour), FamilyExpiresAt: now.Add(24 * time.Hour)}, err: tc.storeErr}
			verifier := &credential.Verifier{Store: db, Audience: publicURL + "/mcp", Clock: clock}
			mux := newWebMux(webDeps{
				store: routeAccountStore{}, sessions: &websession.Manager{Store: routeSessionStore{}, PublicURL: publicURL},
				login: &weblogin.Handler{}, verifier: verifier,
				tools: mcpapi.Config{Clock: clock, Limiter: limits.New(clock)}, loginLimit: newLoginLimiter(clock, nil), publicURL: publicURL,
			})
			server := httptest.NewServer(mux)
			t.Cleanup(server.Close)
			request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL+"/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"guide","arguments":{"country":"vn"}}}`))
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Accept", "application/json, text/event-stream")
			if tc.auth {
				request.Header.Set("Authorization", "Bearer tra_"+strings.Repeat("A", 43))
			}
			response, err := server.Client().Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = response.Body.Close() }()
			body, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatal(err)
			}
			if response.StatusCode != tc.status {
				t.Fatalf("status=%d want=%d body=%s", response.StatusCode, tc.status, body)
			}
			if got := response.Header.Get("Cache-Control"); got != mcpapi.CacheControl {
				t.Errorf("Cache-Control=%q want no-store", got)
			}
			if !tc.auth && db.lookups.Load() != 0 {
				t.Error("unauthenticated request queried the store")
			}
			if tc.auth && db.lookups.Load() != 1 {
				t.Error("request did not check its credential against the store")
			}
			if tc.status == http.StatusUnauthorized && response.Header.Get("WWW-Authenticate") == "" {
				t.Error("missing authentication challenge")
			}
			if tc.status == http.StatusForbidden && !strings.Contains(response.Header.Get("WWW-Authenticate"), `error="insufficient_scope"`) {
				t.Error("missing scope rejection challenge")
			}
			if tc.status == http.StatusOK {
				var message struct {
					Error  json.RawMessage `json:"error"`
					Result struct {
						IsError    bool `json:"isError"`
						Structured struct {
							Guide string `json:"guide"`
						} `json:"structuredContent"`
					} `json:"result"`
				}
				if json.Unmarshal(body, &message) != nil || message.Error != nil || message.Result.IsError || message.Result.Structured.Guide == "" {
					t.Fatalf("guide call did not succeed: %s", body)
				}
			}
		})
	}
}
