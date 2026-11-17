//go:build integration

package app

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/contracttest"
	"danny.vn/trachuan/apps/server/internal/mcpapi"
	"danny.vn/trachuan/apps/server/internal/weblogin"
	"danny.vn/trachuan/apps/server/internal/websession"
	frameworkserve "danny.vn/trachuan/internal/frameworks/serve"
	"github.com/stretchr/testify/require"
)

func frameworkWebTransport(engine *frameworkserve.Engine, auth *frameworkTransportAuth) http.Handler {
	cfg := mcpapi.Config{Frameworks: true, FrameworkResolver: frameworkTransportLease{engine}, FrameworkAccess: frameworkCaller(auth), Limiter: frameworkTransportLimiter{}, Caches: mcpapi.NewCaches(time.Now)}
	return newWebMux(webDeps{store: routeAccountStore{}, sessions: &websession.Manager{Store: routeSessionStore{}, PublicURL: "http://localhost"}, login: &weblogin.Handler{}, tools: cfg, loginLimit: newLoginLimiter(time.Now, nil), publicURL: "http://localhost"})
}

func frameworkWebCall(t *testing.T, handler http.Handler, method, path string, query url.Values, body map[string]any) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	raw := []byte{}
	if body != nil {
		var err error
		raw, err = json.Marshal(body)
		require.NoError(t, err)
	}
	if len(query) > 0 {
		path += "?" + query.Encode()
	}
	r := httptest.NewRequest(method, path, strings.NewReader(string(raw)))
	r.AddCookie(&http.Cookie{Name: "__Host-session", Value: strings.Repeat("A", 43)})
	r.Header.Set("Origin", "http://localhost")
	r.Header.Set("X-CSRF-Token", strings.Repeat("A", 43))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	var value map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &value))
	return w, value
}

func TestFrameworkWeb_LicensedTextInvariant(t *testing.T) {
	c, err := contracttest.Load()
	require.NoError(t, err)
	coverage := contracttest.NewCoverageForTag(c, "frameworks")
	left, _, _ := frameworkFixture(t, false, "20990101")
	right, _, _ := frameworkFixture(t, true, "20990101")
	for _, scopes := range [][]string{nil, {"fw-gamma"}, {"*"}} {
		a := frameworkWebTransport(left, &frameworkTransportAuth{scopes: scopes})
		b := frameworkWebTransport(right, &frameworkTransportAuth{scopes: scopes})
		for _, op := range []struct {
			method, path, actual string
			query                url.Values
			body                 map[string]any
			differs, paged       bool
		}{
			{"GET", "/api/v1/frameworks", "/api/v1/frameworks", nil, nil, false, false},
			{"GET", "/api/v1/frameworks/versions/{handle}", "/api/v1/frameworks/versions/" + url.PathEscape("fw:fw-alpha:1"), nil, nil, false, false},
			{"GET", "/api/v1/frameworks/controls/{handle}", "/api/v1/frameworks/controls/" + url.PathEscape("fw:fw-alpha:1#A-1"), nil, nil, true, true},
			{"POST", "/api/v1/frameworks/search", "/api/v1/frameworks/search", nil, map[string]any{"corpus": "frameworks", "query": "quartz", "limit": 1}, false, true},
			{"POST", "/api/v1/frameworks/search", "/api/v1/frameworks/search", nil, map[string]any{"corpus": "frameworks", "query": "FW-ALPHA A-1"}, true, false},
			{"GET", "/api/v1/frameworks/mappings", "/api/v1/frameworks/mappings", url.Values{"handle": {"fw:fw-alpha:1#A-1"}, "limit": {"200"}}, nil, true, true},
			{"GET", "/api/v1/frameworks/gaps", "/api/v1/frameworks/gaps", nil, nil, true, true},
		} {
			full := len(scopes) > 0 && scopes[0] == "*"
			for range 2 {
				query := url.Values{}
				for k, v := range op.query {
					query[k] = append([]string(nil), v...)
				}
				var body map[string]any
				if op.body != nil {
					body = map[string]any{}
				}
				for k, v := range op.body {
					body[k] = v
				}
				different, pages := false, 0
				for ; pages < 20; pages++ {
					first, value := frameworkWebCall(t, a, op.method, op.actual, query, body)
					second, _ := frameworkWebCall(t, b, op.method, op.actual, query, body)
					require.Equal(t, 200, first.Code)
					require.Equal(t, 200, second.Code)
					coverage.Check(t, op.method, op.path, first)
					coverage.Check(t, op.method, op.path, second)
					if !full {
						require.Equal(t, first.Body.String(), second.Body.String(), op.actual)
					}
					different = different || first.Body.String() != second.Body.String()
					if full && (different || !op.differs) {
						break
					}
					next, _ := value["next_cursor"].(string)
					parameter := "cursor"
					if strings.Contains(op.path, "{handle}") {
						item := value["items"].([]any)[0].(map[string]any)
						next, _ = item["next"].(string)
						parameter = "next"
					}
					if next == "" {
						break
					}
					if op.method == "POST" {
						body[parameter] = next
					} else {
						query.Set(parameter, next)
					}
				}
				require.Less(t, pages, 20)
				if !full && op.paged {
					require.Positive(t, pages, op.actual)
				}
				if full && op.differs {
					require.True(t, different, op.actual)
				}
			}
		}
	}
}

func TestFrameworkWeb_EntitlementFailureAndRevocation(t *testing.T) {
	engine, _, _ := frameworkFixture(t, false, "20990101")
	auth := &frameworkTransportAuth{scopes: []string{"*"}, entitlementErr: errors.New("invented entitlement failure")}
	h := frameworkWebTransport(engine, auth)
	path := "/api/v1/frameworks/controls/" + url.PathEscape("fw:fw-alpha:1#A-1")
	first, reduced := frameworkWebCall(t, h, "GET", path, nil, nil)
	require.Equal(t, 200, first.Code)
	require.Equal(t, true, reduced["access_unavailable"])
	require.NotContains(t, first.Body.String(), "Invented quartz publisher title")
	auth.entitlementErr = nil
	second, full := frameworkWebCall(t, h, "GET", path, nil, nil)
	require.Equal(t, 200, second.Code)
	require.Nil(t, full["access_unavailable"])
	require.Contains(t, second.Body.String(), "Invented quartz publisher title")
	require.NotEqual(t, reduced["access_set_version"], full["access_set_version"])
	auth.scopes = nil
	third, revoked := frameworkWebCall(t, h, "GET", path, nil, nil)
	require.Equal(t, 200, third.Code)
	require.Equal(t, reduced["access_set_version"], revoked["access_set_version"])
	require.NotContains(t, third.Body.String(), "Invented quartz publisher title")
	next := full["items"].([]any)[0].(map[string]any)["next"].(string)
	continued, value := frameworkWebCall(t, h, "GET", path, url.Values{"next": {next}}, nil)
	require.Equal(t, 200, continued.Code)
	require.Equal(t, "invalid_next", value["items"].([]any)[0].(map[string]any)["error"])
	require.Equal(t, 4, auth.lookups)
}
