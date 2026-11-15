package app

import (
	"os"
	"slices"
	"strings"
	"testing"
)

func originConfigEnv(t *testing.T) {
	t.Helper()
	t.Setenv("SERVER_AUTH_DATABASE_URL", "postgres://localhost/auth")
	t.Setenv("SERVER_CORPUS_DATABASE_URL", "postgres://localhost/{database}")
	t.Setenv("SERVER_PUBLIC_URL", "https://example.test")
	t.Setenv("SERVER_GOOGLE_ISSUER", "")
	t.Setenv("SERVER_OAUTH_CLIENTS", "")
	t.Setenv("SERVER_MCP_ORIGINS", "")
	t.Setenv("SERVER_MCP_ORIGIN_MODE", "log")
}

func TestConfig_MCPOriginsInvalid(t *testing.T) {
	originConfigEnv(t)
	for _, raw := range []string{
		"null", "example.test", "https:///", "https://", "https://example.test/",
		"https://example.test/path", "https://example.test?", "https://example.test?q=1",
		"https://example.test#", "https://example.test#fragment", "https://user@example.test",
		"https://example.test:", "https://example.test:bad", "https://example.test:65536",
		"https://example.test:0", "https://example.test,", ",https://example.test",
		"https://example.test,,https://other.test", "https://exa mple.test", "https:example.test",
		"https://::1", "https://2001:db8::1", "https://[invalid]", "https://[127.0.0.1]",
	} {
		t.Run(raw, func(t *testing.T) {
			t.Setenv("SERVER_MCP_ORIGINS", raw)
			if _, err := loadConfig(); err == nil || !strings.Contains(err.Error(), "SERVER_MCP_ORIGINS") {
				t.Fatalf("invalid origin configuration accepted or wrong error: %v", err)
			}
		})
	}
}

func TestConfig_MCPOriginModeInvalid(t *testing.T) {
	originConfigEnv(t)
	for _, raw := range []string{"", "invalid", "LOG", " enforce "} {
		t.Run(raw, func(t *testing.T) {
			t.Setenv("SERVER_MCP_ORIGIN_MODE", raw)
			if _, err := loadConfig(); err == nil || !strings.Contains(err.Error(), "SERVER_MCP_ORIGIN_MODE") {
				t.Fatalf("invalid mode accepted or wrong error: %v", err)
			}
		})
	}
}

func TestConfig_MCPOriginsValues(t *testing.T) {
	originConfigEnv(t)
	for _, tc := range []struct {
		name, raw string
		unset     bool
		want      []string
	}{
		{name: "unset", unset: true},
		{name: "empty"},
		{name: "spaces", raw: " \t "},
		{name: "exact", raw: " https://CLIENT.test:443 , http://localhost:8080 , https://[::1]:8443 ", want: []string{"https://CLIENT.test:443", "http://localhost:8080", "https://[::1]:8443"}},
		{name: "hosts and schemes", raw: "http://client.test,https://192.0.2.1,https://[2001:db8::1],custom://client.test", want: []string{"http://client.test", "https://192.0.2.1", "https://[2001:db8::1]", "custom://client.test"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("SERVER_MCP_ORIGINS", tc.raw)
			if tc.unset {
				unsetOriginEnv(t, "SERVER_MCP_ORIGINS")
			}
			cfg, err := loadConfig()
			if err != nil || !slices.Equal(cfg.mcpOrigins, tc.want) {
				t.Fatalf("origins=%q error=%v", cfg.mcpOrigins, err)
			}
		})
	}
}

func TestConfig_MCPOriginModeValues(t *testing.T) {
	originConfigEnv(t)
	for _, mode := range []string{"unset", "log", "enforce"} {
		t.Run(mode, func(t *testing.T) {
			want := mode
			if mode == "unset" {
				unsetOriginEnv(t, "SERVER_MCP_ORIGIN_MODE")
				want = "enforce"
			} else {
				t.Setenv("SERVER_MCP_ORIGIN_MODE", mode)
			}
			cfg, err := loadConfig()
			if err != nil || cfg.mcpOriginMode != want {
				t.Fatalf("mode=%q error=%v", cfg.mcpOriginMode, err)
			}
		})
	}
}

func unsetOriginEnv(t *testing.T, key string) {
	t.Helper()
	t.Setenv(key, "")
	if err := os.Unsetenv(key); err != nil {
		t.Fatal(err)
	}
}
