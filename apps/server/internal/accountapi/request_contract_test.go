package accountapi

import (
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/contracttest"
)

func testTokenRequestContractDays(t *testing.T, c *contracttest.Contract) {
	for _, tc := range []struct {
		body   string
		status int
	}{
		{`{"name":"test"}`, 201}, {`{"name":"test","days":null}`, 201}, {`{"name":"test","days":1}`, 201}, {`{"name":"test","days":365}`, 201}, {`{"name":"test","days":0}`, 400}, {`{"name":"test","days":366}`, 400}, {`{"name":"test","days":"90"}`, 400},
	} {
		t.Run(tc.body, func(t *testing.T) {
			h, store, _, raw, secret := setup(t)
			w := call(h, raw, secret, "POST", "/api/v1/me/tokens", tc.body)
			if w.Code != tc.status {
				t.Fatal(w.Code)
			}
			if tc.body == `{"name":"test"}` || tc.body == `{"name":"test","days":null}` {
				if len(store.tokens) != 1 || !store.tokens[0].ExpiresAt.Equal(store.now.Add(90*24*time.Hour)) {
					t.Fatal("default expiry is not 90 days")
				}
			}
			err := c.ValidateRequest("POST", "/api/v1/me/tokens", []byte(tc.body))
			if (err == nil) != (tc.status == 201) {
				t.Fatalf("schema validation: %v", err)
			}
		})
	}
}
