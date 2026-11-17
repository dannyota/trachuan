//go:build integration

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/credential"
	"danny.vn/trachuan/apps/server/internal/limits"
	"danny.vn/trachuan/apps/server/internal/mcpapi"
	"danny.vn/trachuan/apps/server/internal/store"
	frameworkserve "danny.vn/trachuan/internal/frameworks/serve"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

type frameworkTransportAuth struct {
	scopes                        []string
	entitlementErr, credentialErr error
	lookups                       int
}

func (s *frameworkTransportAuth) GetCredential(context.Context, string) (store.GetCredentialRow, error) {
	return store.GetCredentialRow{AccountID: 7, ExpiresAt: time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)}, s.credentialErr
}

func (*frameworkTransportAuth) TouchPersonalToken(context.Context, string) error { return nil }

func (*frameworkTransportAuth) GetAccount(context.Context, int64) (store.Account, error) {
	return store.Account{ID: 7, Email: "invented@example.test"}, nil
}

func (s *frameworkTransportAuth) ListLiveFrameworkEntitlementsByEmail(context.Context, string) ([]store.FrameworkEntitlement, error) {
	s.lookups++
	var rows []store.FrameworkEntitlement
	for _, scope := range s.scopes {
		rows = append(rows, store.FrameworkEntitlement{Email: "invented@example.test", Scope: scope})
	}
	return rows, s.entitlementErr
}

type frameworkTransportLease struct{ engine *frameworkserve.Engine }

func (r frameworkTransportLease) ResolveFrameworks(_ context.Context, rel string) (mcpapi.FrameworkLease, error) {
	if rel != "" && rel != r.engine.Release() {
		return mcpapi.FrameworkLease{}, frameworkserve.ErrReleaseExpired
	}
	return mcpapi.FrameworkLease{Engine: r.engine, Done: func() {}}, nil
}

type frameworkTransportLimiter struct{}

func (frameworkTransportLimiter) Allow(int64, limits.Class) (bool, time.Duration) { return true, 0 }

type frameworkBearer struct{ t *testing.T }

func (b frameworkBearer) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer trc_"+strings.Repeat("A", 43))
	response, err := http.DefaultTransport.RoundTrip(r)
	if err == nil && response.Header.Get("Cache-Control") != mcpapi.CacheControl {
		b.t.Errorf("MCP response Cache-Control=%q want no-store", response.Header.Get("Cache-Control"))
	}
	return response, err
}

func frameworkTransport(t *testing.T, engine *frameworkserve.Engine, auth *frameworkTransportAuth, law ...mcpapi.Resolver) (*mcp.ClientSession, *bytes.Buffer) {
	t.Helper()
	var logs bytes.Buffer
	cfg := mcpapi.Config{Frameworks: true, FrameworkResolver: frameworkTransportLease{engine}, FrameworkAccess: frameworkCaller(auth), Limiter: frameworkTransportLimiter{}, Caches: mcpapi.NewCaches(time.Now), Logger: slog.New(slog.NewTextHandler(&logs, nil))}
	if len(law) > 0 {
		cfg.Resolver = law[0]
	}
	verifier := &credential.Verifier{Store: auth, Audience: "https://example.test/mcp", Clock: func() time.Time { return time.Date(2098, 1, 1, 0, 0, 0, 0, time.UTC) }}
	server := httptest.NewServer(mcpapi.NoStore(credential.Middleware(verifier, mcpapi.New(cfg))))
	t.Cleanup(server.Close)
	client := mcp.NewClient(&mcp.Implementation{Name: "invented", Version: "1"}, nil)
	session, err := client.Connect(t.Context(), &mcp.StreamableClientTransport{Endpoint: server.URL, HTTPClient: &http.Client{Transport: frameworkBearer{t: t}}, DisableStandaloneSSE: true, MaxRetries: -1}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, session.Close()) })
	return session, &logs
}

func frameworkWireCall(t *testing.T, session *mcp.ClientSession, name string, args map[string]any) (map[string]any, []byte, bool) {
	t.Helper()
	result, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: args})
	require.NoError(t, err)
	raw, err := json.Marshal(result)
	require.NoError(t, err)
	structured, err := json.Marshal(result.StructuredContent)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)
	require.Equal(t, string(structured), result.Content[0].(*mcp.TextContent).Text)
	var value map[string]any
	require.NoError(t, json.Unmarshal(structured, &value))
	return value, raw, result.IsError
}

func TestFrameworkMCP_EntitlementLookupFailureAndAuthentication(t *testing.T) {
	engine, _, _ := frameworkFixture(t, false, "20990101")
	auth := &frameworkTransportAuth{scopes: []string{"*"}, entitlementErr: errors.New("invented private lookup failure")}
	session, logs := frameworkTransport(t, engine, auth)
	args := map[string]any{"corpus": "frameworks", "items": []any{map[string]any{"handle": "fw:fw-alpha:1#A-1"}}}
	value, raw, failed := frameworkWireCall(t, session, "read", args)
	require.False(t, failed)
	require.Equal(t, true, value["access_unavailable"])
	require.NotEmpty(t, value["access_set_version"])
	require.NotContains(t, string(raw), "Invented quartz publisher title")
	auth.entitlementErr = nil
	next, full, failed := frameworkWireCall(t, session, "read", args)
	require.False(t, failed)
	require.Nil(t, next["access_unavailable"])
	require.NotEqual(t, value["access_set_version"], next["access_set_version"])
	require.Contains(t, string(full), "Invented quartz publisher title")
	require.Equal(t, 2, auth.lookups)
	require.NotContains(t, logs.String(), "quartz")
	require.NotContains(t, logs.String(), "invented private lookup failure")
	denied := &frameworkTransportAuth{credentialErr: credential.ErrUnavailable}
	verifier := &credential.Verifier{Store: denied, Audience: "https://example.test/mcp"}
	handler := mcpapi.NoStore(credential.Middleware(verifier, mcpapi.New(mcpapi.Config{Frameworks: true, FrameworkAccess: frameworkCaller(denied)})))
	r := httptest.NewRequest("POST", "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"corpus_status","arguments":{"corpus":"frameworks"}}}`))
	r.Header.Set("Authorization", "Bearer trc_"+strings.Repeat("A", 43))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	require.Equal(t, http.StatusServiceUnavailable, w.Code)
	require.Zero(t, denied.lookups)
}
