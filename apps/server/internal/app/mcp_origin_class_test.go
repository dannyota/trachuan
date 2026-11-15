package app

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestMCPOriginTracker_CanonicalClasses(t *testing.T) {
	for _, tc := range []struct {
		name   string
		values []string
		want   string
	}{
		{"scheme host and default port", []string{"HTTPS://CLIENT.TEST:443"}, "https://client.test"},
		{"remote http", []string{"http://CLIENT.TEST:80"}, "http://client.test"},
		{"nondefault port", []string{"http://CLIENT.TEST:08080"}, "http://client.test:8080"},
		{"ipv6", []string{"https://[2001:0DB8:0:0:0:0:0:1]:443"}, "https://[2001:db8::1]"},
		{"ipv6 port", []string{"http://[::1]:8080"}, "http://[::1]:8080"},
		{"unicode hostname", []string{"https://BÜCHER.test"}, "https://xn--bcher-kva.test"},
		{"empty", []string{""}, "malformed"},
		{"null", []string{"null"}, "malformed"},
		{"other scheme", []string{"custom://client.test"}, "malformed"},
		{"path", []string{"https://client.test/"}, "malformed"},
		{"empty query", []string{"https://client.test?"}, "malformed"},
		{"empty fragment", []string{"https://client.test#"}, "malformed"},
		{"userinfo", []string{"https://synthetic@client.test"}, "malformed"},
		{"port overflow", []string{"https://client.test:65536"}, "malformed"},
		{"empty port", []string{"https://client.test:"}, "malformed"},
		{"zero port", []string{"https://client.test:0"}, "malformed"},
		{"host absent", []string{"https://:443"}, "malformed"},
		{"bare ipv6", []string{"https://2001:db8::1"}, "malformed"},
		{"invalid ipv6", []string{"https://[synthetic]"}, "malformed"},
		{"ipv6 zone", []string{"https://[fe80::1%25synthetic]"}, "malformed"},
		{"spaces", []string{" https://client.test "}, "malformed"},
		{"control", []string{"https://client.test\nsynthetic"}, "malformed"},
		{"folded", []string{"https://client.test, https://other.test"}, "malformed"},
		{"duplicate headers", []string{"https://client.test", "https://client.test"}, "multiple"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var logs bytes.Buffer
			policy := newMCPOriginPolicy("https://example.test", nil, "log", slog.New(slog.NewJSONHandler(&logs, nil)), time.Now)
			handler := policy.middleware("", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))
			r := httptest.NewRequest("GET", "/mcp", nil)
			r.Header["Origin"] = tc.values
			handler.ServeHTTP(httptest.NewRecorder(), r)
			records := originLogs(t, &logs)
			if len(records) != 1 || records[0]["origin"] != tc.want {
				t.Fatal("logged origin class differs from the expected class")
			}
			if _, stored := policy.tracker.origins[tc.want]; !stored || len(policy.tracker.origins) != 1 {
				t.Fatal("tracker did not key on the logged class")
			}
		})
	}
}
