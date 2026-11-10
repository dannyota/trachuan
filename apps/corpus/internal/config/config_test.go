package config

import (
	"strings"
	"testing"
	"time"
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
		EmbedderURL: "http://127.0.0.1:8092", QueryEmbedderURL: "http://127.0.0.1:8091",
		BuilderTemplate: "corpus_template", PgRestore: "pg_restore",
		DatabaseURL:     "postgres://example",
		TemporalAddress: "127.0.0.1:27233",
		Namespace:       "corpus",
		Artifacts:       "work/artifacts",
		PgDump:          "pg_dump",
		ChinhphuURL:     "https://vanban.chinhphu.vn", ChinhphuFileURL: "https://datafiles.chinhphu.vn", ChinhphuRate: 1, ChinhphuInFlight: 1,
		VBPLFileHosts: "s3-han02.fptcloud.com",
		VBPLURL:       "https://vbpl-bientap-gateway.moj.gov.vn/api/qtdc/public",
		VBPLRate:      16, VBPLInFlight: 16, VBPLFileInFlight: 8, FetchBatchesInFlight: 16, BatchesInFlight: 8, DiskStartGiB: 30, DiskMinGiB: 10,
		HeartbeatEvery: 20 * time.Second, HeartbeatTimeout: 2 * time.Minute,
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
	for _, key := range []string{"CORPUS_VBPL_RPS", "CORPUS_VBPL_IN_FLIGHT", "CORPUS_BATCHES_IN_FLIGHT", "CORPUS_HEARTBEAT_EVERY", "CORPUS_HEARTBEAT_TIMEOUT"} {
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
	values := map[string]string{"CORPUS_DATABASE_URL": "db", "CORPUS_HEARTBEAT_EVERY": "200ms", "CORPUS_HEARTBEAT_TIMEOUT": "2s", "CORPUS_VBPL_RPS": "3.5", "CORPUS_VBPL_IN_FLIGHT": "4", "CORPUS_BATCHES_IN_FLIGHT": "5", "CORPUS_DISK_START_GIB": "4", "CORPUS_DISK_MIN_GIB": "3", "CORPUS_FILE_CACHE": "/cache", "CORPUS_VBPL_FILE_HOSTS": "one.example,two.example"}
	got, err := Load(func(key string) string { return values[key] })
	if err != nil || got.HeartbeatEvery != 200*time.Millisecond || got.HeartbeatTimeout != 2*time.Second || got.VBPLRate != 3.5 || got.VBPLInFlight != 4 || got.BatchesInFlight != 5 || got.DiskStartGiB != 4 || got.DiskMinGiB != 3 || got.FileCache != "/cache" || got.VBPLFileHosts != "one.example,two.example" {
		t.Fatalf("config = %+v, error = %v", got, err)
	}
}

func TestLoad_LoopbackHTTPOffByDefault(t *testing.T) {
	cfg, err := Load(func(key string) string {
		if key == "CORPUS_DATABASE_URL" {
			return "postgres://x"
		}
		return ""
	})
	if err != nil || cfg.VBPLLoopbackHTTP {
		t.Fatalf("loopback=%v err=%v", cfg.VBPLLoopbackHTTP, err)
	}
}

func TestLoad_HeartbeatEveryMustBeBelowTimeout(t *testing.T) {
	env := map[string]string{"CORPUS_DATABASE_URL": "postgres://x", "CORPUS_HEARTBEAT_EVERY": "2m", "CORPUS_HEARTBEAT_TIMEOUT": "2m"}
	if _, err := Load(func(k string) string { return env[k] }); err == nil || !strings.Contains(err.Error(), "CORPUS_HEARTBEAT_EVERY") {
		t.Fatalf("want an error naming CORPUS_HEARTBEAT_EVERY, got %v", err)
	}
}

func TestConfig_EmbedderAndBuilderSettings(t *testing.T) {
	cfg, err := Load(func(key string) string {
		if key == "CORPUS_DATABASE_URL" {
			return "postgres://example"
		}
		return ""
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.EmbedderURL != "http://127.0.0.1:8092" || cfg.QueryEmbedderURL != "http://127.0.0.1:8091" || cfg.BuilderTemplate != "corpus_template" || cfg.PgRestore != "pg_restore" {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
}

func TestLoad_InvalidDiskLimits(t *testing.T) {
	for _, key := range []string{"CORPUS_DISK_START_GIB", "CORPUS_DISK_MIN_GIB"} {
		for _, value := range []string{"0", "-1", "bad"} {
			t.Run(key+"/"+value, func(t *testing.T) {
				_, err := Load(func(k string) string {
					if k == "CORPUS_DATABASE_URL" {
						return "db"
					}
					if k == key {
						return value
					}
					return ""
				})
				if err == nil || !strings.Contains(err.Error(), key) {
					t.Fatalf("error = %v", err)
				}
			})
		}
	}
}

func TestLoad_RejectsDiskStartBelowMinimum(t *testing.T) {
	env := map[string]string{"CORPUS_DATABASE_URL": "db", "CORPUS_DISK_START_GIB": "2", "CORPUS_DISK_MIN_GIB": "3"}
	_, err := Load(func(key string) string { return env[key] })
	if err == nil || !strings.Contains(err.Error(), "CORPUS_DISK_START_GIB") || !strings.Contains(err.Error(), "CORPUS_DISK_MIN_GIB") {
		t.Fatalf("error = %v", err)
	}
}

func TestLoad_FetchAndFileLimits(t *testing.T) {
	for _, tc := range []struct {
		name        string
		values      map[string]string
		file, fetch int
		invalid     string
	}{
		{"defaults", nil, 8, 16, ""},
		{"overrides", map[string]string{"CORPUS_VBPL_FILE_IN_FLIGHT": "3", "CORPUS_FETCH_BATCHES_IN_FLIGHT": "7"}, 3, 7, ""},
		{"invalid file", map[string]string{"CORPUS_VBPL_FILE_IN_FLIGHT": "0"}, 0, 0, "CORPUS_VBPL_FILE_IN_FLIGHT"},
		{"invalid fetch", map[string]string{"CORPUS_FETCH_BATCHES_IN_FLIGHT": "bad"}, 0, 0, "CORPUS_FETCH_BATCHES_IN_FLIGHT"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := Load(func(k string) string {
				if k == "CORPUS_DATABASE_URL" {
					return "db"
				}
				return tc.values[k]
			})
			if tc.invalid != "" {
				if err == nil || !strings.Contains(err.Error(), tc.invalid) {
					t.Fatalf("error = %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if cfg.VBPLFileInFlight != tc.file || cfg.FetchBatchesInFlight != tc.fetch || (tc.name == "defaults" && (cfg.VBPLRate != 16 || cfg.VBPLInFlight != 16)) {
				t.Fatalf("config = %+v", cfg)
			}
		})
	}
}

func TestConfig_ChinhphuDefaultsAndValidation(t *testing.T) {
	cfg, err := Load(func(k string) string {
		if k == "CORPUS_DATABASE_URL" {
			return "db"
		}
		return ""
	})
	if err != nil || cfg.ChinhphuURL != "https://vanban.chinhphu.vn" || cfg.ChinhphuFileURL != "https://datafiles.chinhphu.vn" || cfg.ChinhphuRate != 1 || cfg.ChinhphuInFlight != 1 || cfg.ChinhphuLoopbackHTTP {
		t.Fatalf("defaults = %+v, error = %v", cfg, err)
	}
	for _, key := range []string{"CORPUS_CHINHPHU_RPS", "CORPUS_CHINHPHU_IN_FLIGHT"} {
		t.Run(key, func(t *testing.T) {
			_, err := Load(func(k string) string {
				if k == "CORPUS_DATABASE_URL" {
					return "db"
				}
				if k == key {
					return "0"
				}
				return ""
			})
			if err == nil || !strings.Contains(err.Error(), key) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}
