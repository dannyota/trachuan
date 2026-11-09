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
		VBPLRate:        8, VBPLInFlight: 8, BatchesInFlight: 8,
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

func TestLoad_InvalidLimits(t *testing.T) {
	for _, key := range []string{"CORPUS_VBPL_RPS", "CORPUS_VBPL_IN_FLIGHT", "CORPUS_BATCHES_IN_FLIGHT"} {
		t.Run(key, func(t *testing.T) {
			_, err := Load(func(k string) string {
				if k == "CORPUS_DATABASE_URL" {
					return "db"
				}
				if k == key {
					return "invalid"
				}
				return ""
			})
			if err == nil || !strings.Contains(err.Error(), key) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestLoad_Overrides(t *testing.T) {
	values := map[string]string{"CORPUS_DATABASE_URL": "db", "CORPUS_VBPL_RPS": "3.5", "CORPUS_VBPL_IN_FLIGHT": "4", "CORPUS_BATCHES_IN_FLIGHT": "5", "CORPUS_FILE_CACHE": "/cache"}
	got, err := Load(func(key string) string { return values[key] })
	if err != nil || got.VBPLRate != 3.5 || got.VBPLInFlight != 4 || got.BatchesInFlight != 5 || got.FileCache != "/cache" {
		t.Fatalf("config = %+v, error = %v", got, err)
	}
}
