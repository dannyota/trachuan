package webapi

import (
	"net/http/httptest"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/server/internal/contracttest"
	"danny.vn/trachuan/internal/frameworks/serve"
	"github.com/stretchr/testify/require"
)

func TestFrameworks_OpenAPIContract(t *testing.T) {
	c, err := contracttest.Load()
	require.NoError(t, err)
	coverage := contracttest.NewCoverageForTag(c, "frameworks")
	for _, route := range frameworkRoutes() {
		for _, scenario := range []string{"success", "off", "anonymous", "invalid", "expired", "limited", "unavailable", "csrf", "body"} {
			if (scenario == "csrf" || scenario == "body") && route.method != "POST" {
				continue
			}
			t.Run(route.path+"/"+scenario, func(t *testing.T) {
				h, resolver := frameworkHarness(scenario != "off")
				request := webRequest(route.method, route.actual, route.body)
				want := 200
				switch scenario {
				case "off":
					want = 404
				case "anonymous":
					want = 401
					request.Header.Del("Cookie")
				case "invalid":
					want = 400
					if route.method == "POST" {
						request = webRequest(route.method, route.actual, `{"corpus":"frameworks","query":"invented","extra":true}`)
					} else {
						request.URL.RawQuery += "&extra=true"
					}
				case "expired":
					want = 409
					resolver.err = serve.ErrReleaseExpired
				case "unavailable":
					want = 503
					resolver.err = serve.ErrUnavailable
				case "csrf":
					want = 403
					request.Header.Del("X-CSRF-Token")
				case "body":
					want = 413
					request = webRequest(route.method, route.actual, strings.Repeat("x", (64<<10)+1))
				case "limited":
					want = 429
					count := 120
					if route.method == "POST" {
						count = 60
					}
					for range count {
						h.ServeHTTP(httptest.NewRecorder(), webRequest(route.method, route.actual, route.body))
					}
				}
				w := httptest.NewRecorder()
				h.ServeHTTP(w, request)
				require.Equal(t, want, w.Code)
				require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
				coverage.Check(t, route.method, route.path, w)
			})
		}
	}
	require.Empty(t, coverage.Missing())
}

func TestFrameworks_SearchRequestContract(t *testing.T) {
	c, err := contracttest.Load()
	require.NoError(t, err)
	for _, tc := range []struct {
		body   string
		status int
	}{
		{`{"corpus":"frameworks","query":"invented","limit":1}`, 200},
		{`{"corpus":"frameworks","query":"invented","limit":50}`, 200},
		{`{"corpus":"frameworks","query":"invented","limit":0}`, 400},
		{`{"corpus":"frameworks","query":"invented","limit":51}`, 400},
		{`{"corpus":"law","query":"invented"}`, 400},
		{`{"corpus":"frameworks","query":"invented","extra":true}`, 400},
		{`{"corpus":"frameworks","query":"invented","versions":null}`, 400},
	} {
		h, resolver := frameworkHarness(true)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, webRequest("POST", "/api/v1/frameworks/search", tc.body))
		require.Equal(t, tc.status, w.Code)
		require.Equal(t, tc.status == 200, c.ValidateRequest("POST", "/api/v1/frameworks/search", []byte(tc.body)) == nil)
		if tc.status != 200 {
			require.Zero(t, resolver.calls)
		}
	}
}
