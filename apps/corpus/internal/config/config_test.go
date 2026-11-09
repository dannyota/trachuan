package config

import (
	"strings"
	"testing"
)

func TestLoad_Defaults(t *testing.T) {
	getenv := func(key string) string {
		if key == "CORPUS_DATABASE_URL" {
			return "postgres://example"
		}
		return ""
	}

	got, err := Load(getenv)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	want := Config{
		DatabaseURL:     "postgres://example",
		TemporalAddress: "127.0.0.1:7233",
		Namespace:       "corpus",
		Artifacts:       "work/artifacts",
		PgDump:          "pg_dump",
		VBPLURL:         "https://vbpl-bientap-gateway.moj.gov.vn/api/qtdc/public",
	}
	if got != want {
		t.Errorf("Load() = %+v, want %+v", got, want)
	}
}

func TestLoad_MissingDatabaseURL(t *testing.T) {
	_, err := Load(func(string) string { return "" })
	if err == nil || !strings.Contains(err.Error(), "CORPUS_DATABASE_URL") {
		t.Errorf("Load() error = %v, want CORPUS_DATABASE_URL", err)
	}
}
