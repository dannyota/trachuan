package build

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSourceFile_RejectsWrongHash(t *testing.T) {
	path := filepath.Join(t.TempDir(), "source")
	if err := os.WriteFile(path, []byte("abc"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := checkSourceFile(path, 3, "0000000000000000000000000000000000000000000000000000000000000000"); err == nil {
		t.Fatal("expected hash error")
	}
}
