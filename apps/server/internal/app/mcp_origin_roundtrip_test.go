package app

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMCPOrigin_LoggedValueConfigRoundTrip(t *testing.T) {
	for _, origin := range []string{
		"HTTPS://CLIENT.TEST:443",
		"HTTP://CLIENT.test:080",
		"http://CLIENT.TEST:08080",
		"https://[2001:0DB8:0:0:0:0:0:1]:443",
		"http://[::FFFF:192.0.2.1]:8080",
		"https://XN--BCHER-KVA.test",
		"https://" + strings.Repeat("a.", 94) + "test",
	} {
		t.Run(origin, func(t *testing.T) {
			originConfigEnv(t)
			var logs bytes.Buffer
			mux, _ := originRouteFixture(t, "log", nil, nil, slog.New(slog.NewJSONHandler(&logs, nil)))
			r := httptest.NewRequest("GET", "/mcp", nil)
			r.Header.Set("Origin", origin)
			mux.ServeHTTP(httptest.NewRecorder(), r)
			records := originLogs(t, &logs)
			if len(records) != 1 {
				t.Fatal("missing origin observation")
			}
			logged, ok := records[0]["origin"].(string)
			if !ok {
				t.Fatal("missing logged origin value")
			}
			t.Setenv("SERVER_MCP_ORIGINS", logged)
			t.Setenv("SERVER_MCP_ORIGIN_MODE", "enforce")
			cfg, err := loadConfig()
			if err != nil {
				t.Fatal("logged origin cannot be configured", err)
			}
			mux, _ = originRouteFixture(t, cfg.mcpOriginMode, cfg.mcpOrigins, nil, nil)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, r)
			if w.Code != http.StatusUnauthorized || w.Header().Get("WWW-Authenticate") == "" {
				t.Fatal("original request did not pass the origin check with its logged value configured")
			}
			if logged != origin {
				t.Fatal("observation changed the origin string")
			}
		})
	}
}
