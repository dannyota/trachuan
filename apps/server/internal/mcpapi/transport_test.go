package mcpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/server/internal/credential"
)

func TestGuard_SubscriptionMethodUsesSDKCaseRules(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { called = true; w.WriteHeader(200) })
	v := &credential.Verifier{Store: authStore{}, Audience: "https://example.test/mcp"}
	handler := credential.Middleware(v, guard(next))
	for _, body := range []string{
		`{"jsonrpc":"2.0","id":1,"method":"subscriptions/listen","Method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":1,"Method":"tools/list","method":"subscriptions/listen"}`,
	} {
		r := httptest.NewRequest("POST", "/mcp", strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer trc_"+strings.Repeat("A", 43))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if called || w.Code != 400 {
			t.Fatalf("subscription escaped guard: status=%d", w.Code)
		}
	}
}
