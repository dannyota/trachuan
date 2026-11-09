package build

import (
	"strings"
	"testing"
)

func TestToolError_RedactsConnectionDetails(t *testing.T) {
	stderr := "failed postgres://user:secret@localhost/db and password=secret for user\n" + strings.Repeat("x", 3000)
	got := sanitizeToolStderr(stderr, "secret", "postgres://user:secret@localhost/db")
	if strings.Contains(got, "secret") || strings.Contains(got, "postgres://") || len(got) > 2048 || !strings.Contains(got, "failed") {
		t.Fatalf("stderr was not safely retained")
	}
}

func TestToolError_RedactsKeywordConnectionString(t *testing.T) {
	got := sanitizeToolStderr("failed host=localhost user=bob dbname=example password='pass word'", "pass word", "")
	for _, secret := range []string{"localhost", "bob", "example", "pass word"} {
		if strings.Contains(got, secret) {
			t.Fatalf("connection detail leaked: %q", secret)
		}
	}
}
