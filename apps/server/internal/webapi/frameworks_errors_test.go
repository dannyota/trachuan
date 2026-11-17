package webapi

import (
	"net/http/httptest"
	"testing"

	"danny.vn/trachuan/apps/server/internal/contracttest"
	"danny.vn/trachuan/internal/frameworks/serve"
	"github.com/stretchr/testify/require"
)

func TestFrameworks_EngineErrors(t *testing.T) {
	c, err := contracttest.Load()
	require.NoError(t, err)
	coverage := contracttest.NewCoverageForTag(c, "frameworks")
	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{
		{serve.ErrInvalidRequest, 400, "invalid_request"},
		{serve.ErrInvalidHandle, 400, "invalid_handle"},
		{serve.ErrReleaseExpired, 409, "release_expired"},
		{serve.ErrCursorExpired, 409, "cursor_expired"},
		{serve.ErrRateLimited, 429, "rate_limited"},
		{serve.ErrUnavailable, 503, "unavailable"},
	} {
		t.Run(tc.code, func(t *testing.T) {
			h, resolver := frameworkHarness(true)
			resolver.engine.err = tc.err
			w := httptest.NewRecorder()
			h.ServeHTTP(w, webRequest("POST", "/api/v1/frameworks/search", `{"corpus":"frameworks","query":"invented"}`))
			require.Equal(t, tc.status, w.Code)
			require.Contains(t, w.Body.String(), `"code":"`+tc.code+`"`)
			require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
			if tc.status == 429 {
				require.NotEmpty(t, w.Header().Get("Retry-After"))
			}
			coverage.Check(t, "POST", "/api/v1/frameworks/search", w)
			require.Equal(t, 1, resolver.done)
		})
	}
}
