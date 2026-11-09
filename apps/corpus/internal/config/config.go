// Package config loads corpus configuration from the environment.
package config

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"
)

// Config holds the settings used by corpus commands.
type Config struct {
	EmbedderURL, QueryEmbedderURL                                                  string
	BuilderDatabaseURL, BuilderTemplate, PgRestore, DatabaseDiskPath, EvalFixtures string
	DatabaseURL                                                                    string
	TemporalAddress                                                                string
	Namespace                                                                      string
	Artifacts                                                                      string
	CheckpointDir                                                                  string
	PgDump                                                                         string
	VBPLURL                                                                        string
	VBPLFileHosts                                                                  string
	// VBPLLoopbackHTTP permits plain HTTP downloads from 127.0.0.1 only; tests use it.
	VBPLLoopbackHTTP     bool
	VBPLRate             float64
	VBPLInFlight         int
	VBPLFileInFlight     int
	FetchBatchesInFlight int
	BatchesInFlight      int
	DiskStartGiB         int
	DiskMinGiB           int
	FileCache            string
	HeartbeatEvery       time.Duration
	HeartbeatTimeout     time.Duration
}

// Load reads configuration using getenv and requires a database URL.
func Load(getenv func(string) string) (Config, error) {
	cfg := Config{
		EmbedderURL:        defaultValue(getenv("CORPUS_EMBEDDER_URL"), "http://127.0.0.1:8092"),
		QueryEmbedderURL:   defaultValue(getenv("CORPUS_QUERY_EMBEDDER_URL"), "http://127.0.0.1:8091"),
		BuilderDatabaseURL: getenv("CORPUS_BUILDER_DATABASE_URL"),
		BuilderTemplate:    defaultValue(getenv("CORPUS_BUILDER_TEMPLATE"), "corpus_template"),
		PgRestore:          defaultValue(getenv("CORPUS_PG_RESTORE"), "pg_restore"),
		DatabaseDiskPath:   getenv("CORPUS_DATABASE_DISK_PATH"),
		EvalFixtures:       getenv("CORPUS_EVAL_FIXTURES"),
		DatabaseURL:        getenv("CORPUS_DATABASE_URL"),
		TemporalAddress:    defaultValue(getenv("CORPUS_TEMPORAL_ADDRESS"), "127.0.0.1:7233"),
		Namespace:          defaultValue(getenv("CORPUS_TEMPORAL_NAMESPACE"), "corpus"),
		Artifacts:          defaultValue(getenv("CORPUS_ARTIFACTS"), "work/artifacts"),
		CheckpointDir:      getenv("CORPUS_CHECKPOINT_DIR"),
		PgDump:             defaultValue(getenv("CORPUS_PG_DUMP"), "pg_dump"),
		FileCache:          getenv("CORPUS_FILE_CACHE"),
		VBPLFileHosts:      defaultValue(getenv("CORPUS_VBPL_FILE_HOSTS"), "s3-han02.fptcloud.com"),
		VBPLLoopbackHTTP:   getenv("CORPUS_VBPL_LOOPBACK_HTTP") == "true",
		VBPLURL:            defaultValue(getenv("CORPUS_VBPL_URL"), "https://vbpl-bientap-gateway.moj.gov.vn/api/qtdc/public"),
	}
	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("CORPUS_DATABASE_URL is required")
	}
	rateText := defaultValue(getenv("CORPUS_VBPL_RPS"), "16")
	rate, err := strconv.ParseFloat(rateText, 64)
	if err != nil || rate <= 0 || math.IsNaN(rate) || math.IsInf(rate, 0) {
		return Config{}, fmt.Errorf("invalid CORPUS_VBPL_RPS %q", rateText)
	}
	cfg.VBPLRate = rate
	for _, setting := range []struct {
		key, fallback string
		target        *int
	}{{"CORPUS_VBPL_IN_FLIGHT", "16", &cfg.VBPLInFlight}, {"CORPUS_VBPL_FILE_IN_FLIGHT", "8", &cfg.VBPLFileInFlight}, {"CORPUS_FETCH_BATCHES_IN_FLIGHT", "16", &cfg.FetchBatchesInFlight}, {"CORPUS_BATCHES_IN_FLIGHT", "8", &cfg.BatchesInFlight}, {"CORPUS_DISK_START_GIB", "30", &cfg.DiskStartGiB}, {"CORPUS_DISK_MIN_GIB", "10", &cfg.DiskMinGiB}} {
		value := defaultValue(getenv(setting.key), setting.fallback)
		n, parseErr := strconv.Atoi(value)
		if parseErr != nil || n <= 0 || ((setting.key == "CORPUS_DISK_START_GIB" || setting.key == "CORPUS_DISK_MIN_GIB") && uint64(n) > ^uint64(0)>>30) {
			return Config{}, fmt.Errorf("invalid %s %q", setting.key, value)
		}
		*setting.target = n
	}
	if cfg.DiskStartGiB < cfg.DiskMinGiB {
		return Config{}, fmt.Errorf("CORPUS_DISK_START_GIB must be at least CORPUS_DISK_MIN_GIB")
	}
	for _, setting := range []struct {
		key, fallback string
		target        *time.Duration
	}{{"CORPUS_HEARTBEAT_EVERY", "20s", &cfg.HeartbeatEvery}, {"CORPUS_HEARTBEAT_TIMEOUT", "2m", &cfg.HeartbeatTimeout}} {
		value := defaultValue(getenv(setting.key), setting.fallback)
		duration, parseErr := time.ParseDuration(value)
		if parseErr != nil || duration <= 0 {
			return Config{}, fmt.Errorf("invalid %s %q", setting.key, value)
		}
		*setting.target = duration
	}
	if cfg.HeartbeatEvery >= cfg.HeartbeatTimeout {
		return Config{}, fmt.Errorf("CORPUS_HEARTBEAT_EVERY (%s) must be below CORPUS_HEARTBEAT_TIMEOUT (%s)", cfg.HeartbeatEvery, cfg.HeartbeatTimeout)
	}
	return cfg, nil
}

func defaultValue(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
