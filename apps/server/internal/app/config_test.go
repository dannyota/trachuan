package app

import (
	"context"
	"io"
	"strings"
	"testing"
)

func TestConfig_RequiredAndDefaults(t *testing.T) {
	for _, key := range []string{"SERVER_LISTEN", "SERVER_AUTH_DATABASE_URL", "SERVER_CORPUS_DATABASE_URL", "SERVER_QUERY_EMBEDDER_URL", "SERVER_PUBLIC_URL"} {
		t.Setenv(key, "")
	}
	if _, err := loadConfig(); err == nil {
		t.Fatal("missing required configuration accepted")
	}
	t.Setenv("SERVER_AUTH_DATABASE_URL", "postgres://localhost/auth")
	t.Setenv("SERVER_CORPUS_DATABASE_URL", "postgres://localhost/{database}")
	t.Setenv("SERVER_PUBLIC_URL", "http://localhost:8080")
	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.listen != "127.0.0.1:8080" || cfg.embedderURL != "http://127.0.0.1:8091" || cfg.audience != "http://localhost:8080/mcp" {
		t.Fatal("unexpected defaults")
	}
}

func TestConfig_PublicURLOrigin(t *testing.T) {
	t.Setenv("SERVER_AUTH_DATABASE_URL", "postgres://localhost/auth")
	t.Setenv("SERVER_CORPUS_DATABASE_URL", "postgres://localhost/{database}")
	for _, tt := range []struct{ name, base, audience string }{
		{"https", "https://trachuan.vn", "https://trachuan.vn/mcp"},
		{"https slash port", "https://trachuan.vn:8443/", "https://trachuan.vn:8443/mcp"},
		{"localhost", "http://localhost:8080", "http://localhost:8080/mcp"},
		{"ipv4", "http://127.0.0.1", "http://127.0.0.1/mcp"},
		{"ipv4 loopback range", "http://127.0.0.2", "http://127.0.0.2/mcp"},
		{"ipv6", "http://[::1]:8080/", "http://[::1]:8080/mcp"},
		{"path", "https://trachuan.vn/prefix", ""},
		{"double slash", "https://trachuan.vn//", ""},
		{"query", "https://trachuan.vn/?x=1", ""},
		{"empty query", "https://trachuan.vn/?", ""},
		{"fragment", "https://trachuan.vn/#x", ""},
		{"empty fragment", "https://trachuan.vn/#", ""},
		{"userinfo", "https://user@trachuan.vn", ""},
		{"remote http", "http://trachuan.vn", ""},
		{"other scheme", "ftp://localhost", ""},
		{"no host", "https:///", ""},
		{"loopback lookalike", "http://localhost.example", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("SERVER_PUBLIC_URL", tt.base)
			cfg, err := loadConfig()
			if tt.audience == "" {
				if err == nil {
					t.Fatal("invalid origin accepted")
				}
				return
			}
			if err != nil || cfg.audience != tt.audience {
				t.Fatalf("audience=%q error=%v", cfg.audience, err)
			}
		})
	}
}

func TestConfig_CorpusURLTemplate(t *testing.T) {
	t.Setenv("SERVER_AUTH_DATABASE_URL", "postgres://localhost/auth")
	t.Setenv("SERVER_PUBLIC_URL", "https://example.test")
	for _, tt := range []struct {
		name, template string
		valid          bool
	}{
		{"path", "postgres://reader@localhost/{database}", true},
		{"query", "postgres://localhost/{database}?sslmode=disable", true},
		{"missing", "postgres://localhost/corpus", false},
		{"duplicate", "postgres://localhost/{database}?name={database}", false},
		{"host", "postgres://{database}/corpus", false},
		{"username", "postgres://{database}@localhost/corpus", false},
		{"partial", "postgres://localhost/prefix_{database}", false},
		{"nested", "postgres://localhost/prefix/{database}", false},
		{"query only", "postgres://localhost/corpus?dbname={database}", false},
		{"db override", "postgres://localhost/{database}?dbname=other", false},
		{"escaped path, placeholder in query", "postgres://localhost/%7Bdatabase%7D?application_name={database}", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("SERVER_CORPUS_DATABASE_URL", tt.template)
			_, err := loadConfig()
			if (err == nil) != tt.valid {
				t.Fatalf("valid=%t error=%v", tt.valid, err)
			}
			if err != nil && strings.Contains(err.Error(), tt.template) {
				t.Fatal("error exposed URL")
			}
		})
	}
}

func TestConfig_WebRequiresGoogleAndPublicURL(t *testing.T) {
	t.Setenv("SERVER_AUTH_DATABASE_URL", "postgres://localhost/auth")
	t.Setenv("SERVER_CORPUS_DATABASE_URL", "postgres://localhost/{database}")
	for _, tt := range []struct{ name, id, secret, public string }{
		{"client id", "", "configured", "http://localhost:8080"},
		{"client secret", "configured", "", "http://localhost:8080"},
		{"public URL", "configured", "configured", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GOOGLE_CLIENT_ID", tt.id)
			t.Setenv("GOOGLE_CLIENT_SECRET", tt.secret)
			t.Setenv("SERVER_PUBLIC_URL", tt.public)
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if err := run(ctx, []string{"serve"}, io.Discard, io.Discard); err == nil || err.Error() != "invalid or missing server configuration" {
				t.Fatalf("expected configuration error, got %v", err)
			}
		})
	}
}

func TestConfig_PreregisteredFileOptional(t *testing.T) {
	t.Setenv("SERVER_AUTH_DATABASE_URL", "postgres://localhost/auth")
	t.Setenv("SERVER_CORPUS_DATABASE_URL", "postgres://localhost/{database}")
	t.Setenv("SERVER_PUBLIC_URL", "http://localhost:8080")
	t.Setenv("SERVER_OAUTH_CLIENTS", "")
	if _, err := loadConfig(); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/", "../../../../docs/architecture.md"} {
		t.Setenv("SERVER_OAUTH_CLIENTS", path)
		if _, err := loadConfig(); err == nil || err.Error() != "invalid preregistered OAuth clients" {
			t.Fatalf("error=%v", err)
		}
	}
}

func TestConfig_NormalizesPublicOrigin(t *testing.T) {
	t.Setenv("SERVER_AUTH_DATABASE_URL", "postgres://localhost/auth")
	t.Setenv("SERVER_CORPUS_DATABASE_URL", "postgres://localhost/{database}")
	for _, tt := range []struct{ raw, origin string }{
		{"HTTPS://TraChuan.VN:443/", "https://trachuan.vn"},
		{"HTTP://LOCALHOST:80/", "http://localhost"},
		{"HTTPS://EXAMPLE.TEST:8443/", "https://example.test:8443"},
		{"http://[::1]:80/", "http://[::1]"},
	} {
		t.Run(tt.raw, func(t *testing.T) {
			t.Setenv("SERVER_PUBLIC_URL", tt.raw)
			cfg, err := loadConfig()
			if err != nil || cfg.publicURL != tt.origin || cfg.audience != tt.origin+"/mcp" {
				t.Fatalf("origin=%q audience=%q err=%v", cfg.publicURL, cfg.audience, err)
			}
		})
	}
}
