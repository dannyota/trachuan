package app

import (
	"os"
	"testing"
)

func TestConfig_CompactRelatedInvalid(t *testing.T) {
	t.Setenv("SERVER_AUTH_DATABASE_URL", "postgres://localhost/auth")
	t.Setenv("SERVER_CORPUS_DATABASE_URL", "postgres://localhost/{database}")
	t.Setenv("SERVER_PUBLIC_URL", "http://localhost:8080")
	for _, raw := range []string{"invalid", ""} {
		t.Setenv("SERVER_COMPACT_RELATED", raw)
		if _, err := loadConfig(); err == nil {
			t.Fatalf("invalid compact related setting %q accepted", raw)
		}
	}
}

func TestConfig_CompactRelatedValues(t *testing.T) {
	t.Setenv("SERVER_AUTH_DATABASE_URL", "postgres://localhost/auth")
	t.Setenv("SERVER_CORPUS_DATABASE_URL", "postgres://localhost/{database}")
	t.Setenv("SERVER_PUBLIC_URL", "http://localhost:8080")
	for _, tc := range []struct {
		raw  string
		omit bool
	}{{"unset", false}, {"keep", false}, {"omit", true}} {
		t.Run(tc.raw, func(t *testing.T) {
			t.Setenv("SERVER_COMPACT_RELATED", tc.raw)
			if tc.raw == "unset" {
				if err := os.Unsetenv("SERVER_COMPACT_RELATED"); err != nil {
					t.Fatal(err)
				}
			}
			cfg, err := loadConfig()
			if err != nil || cfg.omitCompactRelated != tc.omit {
				t.Fatalf("omit=%v err=%v", cfg.omitCompactRelated, err)
			}
		})
	}
}
