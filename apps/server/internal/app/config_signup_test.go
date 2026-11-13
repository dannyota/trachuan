package app

import (
	"context"
	"io"
	"os"
	"reflect"
	"testing"
)

func TestConfig_SignupEmails(t *testing.T) {
	t.Setenv("SERVER_AUTH_DATABASE_URL", "postgres://localhost/auth")
	t.Setenv("SERVER_CORPUS_DATABASE_URL", "postgres://localhost/{database}")
	t.Setenv("SERVER_PUBLIC_URL", "http://localhost:8080")
	t.Setenv("SERVER_GOOGLE_ISSUER", "")
	t.Setenv("SERVER_OAUTH_CLIENTS", "")
	for _, tc := range []struct {
		name, raw      string
		unset, invalid bool
		want           []string
	}{
		{name: "unset", unset: true},
		{name: "normalized", raw: " Owner@Example.Invalid , SECOND@example.invalid ", want: []string{"owner@example.invalid", "second@example.invalid"}},
		{name: "empty", invalid: true},
		{name: "whitespace", raw: " \t ", invalid: true},
		{name: "empty entry", raw: "a@b,,c@d", invalid: true},
		{name: "blank entry", raw: "a@b, ,c@d", invalid: true},
		{name: "leading empty", raw: ",a@b", invalid: true},
		{name: "trailing empty", raw: "a@b,", invalid: true},
		{name: "missing at", raw: "not-an-email", invalid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("SERVER_SIGNUP_EMAILS", tc.raw)
			if tc.unset {
				if err := os.Unsetenv("SERVER_SIGNUP_EMAILS"); err != nil {
					t.Fatal(err)
				}
			}
			cfg, err := loadConfig()
			if tc.invalid {
				if err == nil || err.Error() != "invalid or missing server configuration" {
					t.Fatalf("expected generic configuration error, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(cfg.signupEmails, tc.want) {
				t.Fatal("sign-up emails not normalized or unset list not open")
			}
		})
	}
}

func TestConfig_InvalidSignupStopsBeforeDatabase(t *testing.T) {
	t.Setenv("SERVER_AUTH_DATABASE_URL", "://invalid")
	t.Setenv("SERVER_CORPUS_DATABASE_URL", "postgres://localhost/{database}")
	t.Setenv("SERVER_PUBLIC_URL", "https://example.invalid")
	t.Setenv("SERVER_GOOGLE_ISSUER", "")
	t.Setenv("SERVER_OAUTH_CLIENTS", "")
	t.Setenv("GOOGLE_CLIENT_ID", "fixture")
	t.Setenv("GOOGLE_CLIENT_SECRET", "fixture")
	t.Setenv("SERVER_SIGNUP_EMAILS", "")
	err := run(context.Background(), []string{"serve"}, io.Discard, io.Discard)
	if err == nil || err.Error() != "invalid or missing server configuration" {
		t.Fatalf("expected configuration error before database setup, got %v", err)
	}
}
