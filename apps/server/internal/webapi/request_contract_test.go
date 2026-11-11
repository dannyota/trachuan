package webapi

import (
	"strings"
	"testing"

	"danny.vn/trachuan/apps/server/internal/contracttest"
)

func testRequestContractBoundaries(t *testing.T, c *contracttest.Contract) {
	for _, tc := range []struct {
		path, body string
		status     int
	}{
		{"/api/v1/search", `{"country":"vn","query":"ngân hàng","limit":1}`, 200},
		{"/api/v1/search", `{"country":"vn","query":"ngân hàng","limit":50}`, 200},
		{"/api/v1/search", `{"country":"vn","query":"ngân hàng","limit":0}`, 400},
		{"/api/v1/search", `{"country":"vn","query":"ngân hàng","limit":51}`, 400},
		{"/api/v1/search", `{"country":"vn","query":"ngân hàng","limit":50.0000000000000001}`, 400},
		{"/api/v1/search", `{"country":"us","query":"ngân hàng"}`, 400},
		{"/api/v1/read", `{"country":"vn","items":[` + strings.TrimSuffix(strings.Repeat(`{"handle":"vn:1"},`, 8), ",") + `]}`, 200},
		{"/api/v1/read", `{"country":"vn","items":[` + strings.TrimSuffix(strings.Repeat(`{"handle":"vn:1"},`, 9), ",") + `]}`, 400},
	} {
		t.Run(tc.body, func(t *testing.T) {
			w := webCall(newHarness(t), "POST", tc.path, tc.body)
			if w.Code != tc.status {
				t.Fatalf("status %d", w.Code)
			}
			err := c.ValidateRequest("POST", tc.path, []byte(tc.body))
			if (err == nil) != (tc.status == 200) {
				t.Fatalf("schema validation: %v", err)
			}
		})
	}
}
