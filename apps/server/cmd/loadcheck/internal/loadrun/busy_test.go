package loadrun

import (
	"testing"
	"time"
)

func TestBusy_PageAndAPI(t *testing.T) {
	for _, tc := range []struct {
		name   string
		page   bool
		status int
		retry  time.Duration
		body   string
		want   bool
	}{
		{"page 503", true, 503, 0, "", true},
		{"render admission", true, 429, time.Second, "", true},
		{"boundary", true, 429, 5 * time.Second, "", true},
		{"address limit", true, 429, 60 * time.Second, "", false},
		{"zero retry", true, 429, 0, "", true},
		{"missing retry", true, 429, -1, "", false},
		{"API busy", false, 503, 5 * time.Second, `{"error":{"code":"busy"}}`, true},
		{"API unavailable", false, 503, 5 * time.Second, `{"error":{"code":"unavailable"}}`, false},
		{"API 429", false, 429, time.Second, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := isBusy(tc.page, tc.status, tc.retry, []byte(tc.body)); got != tc.want {
				t.Fatalf("busy = %v", got)
			}
		})
	}
}
