// Package config loads corpus configuration from the environment.
package config

import "errors"

// Config holds the settings used by corpus commands.
type Config struct {
	DatabaseURL     string
	TemporalAddress string
	Namespace       string
	Artifacts       string
	CheckpointDir   string
	PgDump          string
}

// Load reads configuration using getenv and requires a database URL.
func Load(getenv func(string) string) (Config, error) {
	cfg := Config{
		DatabaseURL:     getenv("CORPUS_DATABASE_URL"),
		TemporalAddress: defaultValue(getenv("CORPUS_TEMPORAL_ADDRESS"), "127.0.0.1:7233"),
		Namespace:       defaultValue(getenv("CORPUS_TEMPORAL_NAMESPACE"), "corpus"),
		Artifacts:       defaultValue(getenv("CORPUS_ARTIFACTS"), "work/artifacts"),
		CheckpointDir:   getenv("CORPUS_CHECKPOINT_DIR"),
		PgDump:          defaultValue(getenv("CORPUS_PG_DUMP"), "pg_dump"),
	}
	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("CORPUS_DATABASE_URL is required")
	}
	return cfg, nil
}

func defaultValue(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
