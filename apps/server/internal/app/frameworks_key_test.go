package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfig_FrameworksKeyFailsClosed(t *testing.T) {
	t.Setenv("SERVER_AUTH_DATABASE_URL", "postgres://localhost/auth")
	t.Setenv("SERVER_CORPUS_DATABASE_URL", "postgres://localhost/{database}")
	t.Setenv("SERVER_PUBLIC_URL", "http://localhost:8080")
	t.Setenv("SERVER_FRAMEWORKS", "true")
	for _, size := range []int{0, 31, 32, 64} {
		path := filepath.Join(t.TempDir(), "key")
		if err := os.WriteFile(path, []byte(strings.Repeat("s", size)), 0600); err != nil {
			t.Fatal(err)
		}
		t.Setenv("SERVER_FRAMEWORKS_KEY_FILE", path)
		_, err := loadConfig()
		if (err != nil) != (size < 32) {
			t.Fatalf("size=%d error=%v", size, err)
		}
	}
	for _, path := range []string{"", filepath.Join(t.TempDir(), "missing"), t.TempDir()} {
		t.Setenv("SERVER_FRAMEWORKS_KEY_FILE", path)
		if _, err := loadConfig(); err == nil || err.Error() != "invalid or unreadable SERVER_FRAMEWORKS_KEY_FILE" {
			t.Fatalf("missing, unreadable or nonregular file: %v", err)
		}
	}
	t.Setenv("SERVER_FRAMEWORKS", "false")
	if _, err := loadConfig(); err != nil {
		t.Fatalf("disabled feature read key: %v", err)
	}
}
