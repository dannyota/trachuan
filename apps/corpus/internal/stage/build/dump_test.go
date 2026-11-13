package build

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
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

func TestPgDump_UsesZstdCustomArchive(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "pg_dump")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$0.args\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(dir, "corpus.dump")
	cfg := Config{BuilderURL: "postgres://localhost/postgres", PgDump: script}
	if err := pgDump(context.Background(), cfg, "corpus_test", output); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(script + ".args")
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	want := []string{"-Fc", "-Z", "zstd:1", "--no-owner", "--no-privileges", "--no-comments", "--dbname", "corpus_test", "--file", output}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("arguments = %q, want %q", got, want)
	}
}
