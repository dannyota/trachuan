package app

import (
	"context"
	"io"
	"strings"
	"testing"
)

func TestAccountConfig_RequiredKeyBeforePool(t *testing.T) {
	t.Setenv("SERVER_AUTH_DATABASE_URL", "://invalid")
	t.Setenv("SERVER_CORPUS_DATABASE_URL", "postgres://localhost/{database}")
	t.Setenv("SERVER_PUBLIC_URL", "http://localhost:8080")
	t.Setenv("GOOGLE_CLIENT_ID", "synthetic")
	t.Setenv("GOOGLE_CLIENT_SECRET", "synthetic")
	t.Setenv("SERVER_ACCOUNT_KEYS_FILE", "")
	err := run(context.Background(), []string{"serve"}, io.Discard, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "SERVER_ACCOUNT_KEYS_FILE") {
		t.Fatalf("missing key not rejected: %v", err)
	}
}
func TestMailConfig_GuardsAndDefaults(t *testing.T) {
	for _, tc := range []struct {
		public, listen, smtp, capture string
		valid                         bool
	}{
		{"http://localhost:8080", "127.0.0.1:8080", "", "", true},
		{"http://localhost:8080", "127.0.0.1:8080", "", "capture", true},
		{"https://example.test", "127.0.0.1:8080", "", "capture", false},
		{"http://localhost:8080", "0.0.0.0:8080", "", "capture", false},
		{"http://LOCALHOST:8080", "127.0.0.1:8080", "", "capture", false},
		{"http://localhost:8080", "127.0.0.1:8080", "smtp.example.test", "capture", false},
		{"https://example.test", "127.0.0.1:8080", "smtp.example.test", "", true},
	} {
		t.Setenv("SERVER_SMTP_HOST", tc.smtp)
		t.Setenv("SERVER_MAIL_CAPTURE_DIR", tc.capture)
		t.Setenv("SERVER_SMTP_PORT", "")
		t.Setenv("SERVER_SMTP_USERNAME", "synthetic")
		t.Setenv("SERVER_SMTP_PASSWORD", "synthetic")
		t.Setenv("SERVER_MAIL_FROM", "")
		cfg, err := loadMailConfig(tc.public, tc.listen)
		if (err == nil) != tc.valid {
			t.Fatalf("valid=%v err=%v", tc.valid, err)
		}
		if err == nil && (cfg.smtp.Port != "587" || cfg.smtp.From != "no-reply@trachuan.vn") {
			t.Fatal("wrong defaults")
		}
	}
}

func TestMailConfig_RequiresSMTPOutsideLoopback(t *testing.T) {
	for _, deployment := range []struct {
		name, public, listen string
		local                bool
	}{
		{"remote public", "https://example.test", "127.0.0.1:8080", false},
		{"all interfaces", "http://localhost:8080", "0.0.0.0:8080", false},
		{"uppercase public", "http://LOCALHOST:8080", "127.0.0.1:8080", false},
		{"mapped address", "http://[::ffff:127.0.0.1]:8080", "127.0.0.1:8080", false},
		{"localhost", "http://localhost:8080", "localhost:8080", true},
		{"loopback range", "http://127.0.0.2:8080", "[::1]:8080", true},
	} {
		t.Run(deployment.name, func(t *testing.T) {
			t.Setenv("SERVER_MAIL_CAPTURE_DIR", "")
			t.Setenv("SERVER_SMTP_PORT", "")
			t.Setenv("SERVER_MAIL_FROM", "")
			for _, credentials := range []struct{ name, host, user, password string }{
				{"disabled", "", "", ""},
				{"missing host", "", "synthetic", "synthetic"},
				{"missing username", "smtp.example.test", "", "synthetic"},
				{"missing password", "smtp.example.test", "synthetic", ""},
				{"configured", "smtp.example.test", "synthetic", "synthetic"},
			} {
				t.Run(credentials.name, func(t *testing.T) {
					t.Setenv("SERVER_SMTP_HOST", credentials.host)
					t.Setenv("SERVER_SMTP_USERNAME", credentials.user)
					t.Setenv("SERVER_SMTP_PASSWORD", credentials.password)
					_, err := loadMailConfig(deployment.public, deployment.listen)
					valid := credentials.name == "configured" || deployment.local && credentials.host == ""
					if (err == nil) != valid {
						t.Errorf("valid=%v err=%v", valid, err)
					}
					if err != nil && !strings.Contains(err.Error(), "SERVER_SMTP_") {
						t.Error("missing SMTP variable names")
					}
				})
			}
		})
	}
}
