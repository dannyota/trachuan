package app

import "testing"

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
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
}
