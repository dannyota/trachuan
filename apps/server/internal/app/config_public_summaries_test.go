package app

import (
	"os"
	"testing"
)

func TestConfig_PublicSummaries(t *testing.T) {
	t.Setenv("SERVER_AUTH_DATABASE_URL", "postgres://localhost/auth")
	t.Setenv("SERVER_CORPUS_DATABASE_URL", "postgres://localhost/{database}")
	t.Setenv("SERVER_PUBLIC_URL", "http://localhost:8080")
	t.Setenv("SERVER_PUBLIC_SUMMARIES", "false")
	if err := os.Unsetenv("SERVER_PUBLIC_SUMMARIES"); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadConfig()
	if err != nil || cfg.publicSummaries {
		t.Fatalf("default: %v %v", cfg.publicSummaries, err)
	}
	for _, value := range []string{"true", "false", "", "1", "TRUE", " true"} {
		t.Setenv("SERVER_PUBLIC_SUMMARIES", value)
		cfg, err = loadConfig()
		if value == "true" || value == "false" {
			if err != nil || cfg.publicSummaries != (value == "true") {
				t.Fatalf("boolean %q: %v", value, err)
			}
		} else if err == nil || err.Error() != "invalid SERVER_PUBLIC_SUMMARIES" {
			t.Fatalf("invalid %q accepted: %v", value, err)
		}
	}
}
