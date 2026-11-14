package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkerRefusesMissingSourceDirectories(t *testing.T) {
	existing := t.TempDir()
	file := filepath.Join(existing, "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, ocr, cache, want string }{
		{"ocr unset", "", existing, "CORPUS_OCR_STORE"},
		{"cache unset", existing, "", "CORPUS_FILE_CACHE"},
		{"ocr missing", filepath.Join(existing, "absent"), existing, "CORPUS_OCR_STORE"},
		{"cache not a directory", existing, file, "CORPUS_FILE_CACHE"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CORPUS_DATABASE_URL", "postgres://unused@127.0.0.1:1/unused")
			t.Setenv("CORPUS_OCR_STORE", tc.ocr)
			t.Setenv("CORPUS_FILE_CACHE", tc.cache)
			var stderr bytes.Buffer
			if code := runWorker(context.Background(), nil, &bytes.Buffer{}, &stderr); code != 1 {
				t.Fatalf("exit %d, want 1", code)
			}
			if !strings.Contains(stderr.String(), tc.want) {
				t.Fatalf("stderr %q does not name %s", stderr.String(), tc.want)
			}
		})
	}
}
