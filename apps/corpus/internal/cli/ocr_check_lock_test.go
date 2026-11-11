package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestCheck_LockExcludesConcurrentInvocations(t *testing.T) {
	root := t.TempDir()
	first, err := lockOCRStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer first()
	second, err := lockOCRStore(root)
	if second != nil {
		second()
	}
	if err == nil {
		t.Fatal("two check invocations acquired the same store")
	}
	first()
	third, err := lockOCRStore(root)
	if err != nil {
		t.Fatal("released lock could not be acquired")
	}
	third()
}

func TestCheck_CLIRefusesSecondInvocation(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CORPUS_OCR_STORE", root)
	t.Setenv("CORPUS_DATABASE_URL", "")
	unlock, err := lockOCRStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), []string{"ocr", "check", "vn", "20990101", "--max-usd", "1"}, &stdout, &stderr)
	if code != 1 || !strings.Contains(stderr.String(), "another OCR check command holds this store") {
		t.Fatalf("concurrent command reached its dependencies: status=%d error=%s", code, stderr.String())
	}
}
