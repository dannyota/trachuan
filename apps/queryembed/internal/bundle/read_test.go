package bundle

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestReadVerified_ReturnsVerifiedSnapshot(t *testing.T) {
	dir, m := fixture(t)
	want, err := os.ReadFile(filepath.Join(dir, QueryModelPath))
	if err != nil {
		t.Fatal(err)
	}
	got, err := m.ReadVerified(dir, QueryModelPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, QueryModelPath), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("model bytes differ from verified snapshot")
	}
	if _, err := m.ReadVerified(dir, QueryModelPath); err == nil {
		t.Fatal("accepted changed model")
	}
}

func TestReadVerified_RejectsInvalidFile(t *testing.T) {
	for _, change := range []string{"short", "long", "negative", "hash", "missing", "unlisted", "traversal", "absolute", "duplicate", "symlink", "symlink directory", "directory", "fifo"} {
		t.Run(change, func(t *testing.T) {
			dir, m := fixture(t)
			p := QueryModelPath
			name := filepath.Join(dir, p)
			switch change {
			case "short":
				m.Files[0].Size++
			case "long":
				m.Files[0].Size--
			case "negative":
				m.Files[0].Size = -1
			case "hash":
				m.Files[0].SHA256 = strings.Repeat("0", 64)
			case "unlisted":
				p = "absent"
			case "traversal", "absolute", "duplicate":
				f := m.Files[0]
				switch change {
				case "traversal":
					f.Path = "../escape"
				case "absolute":
					f.Path = "/escape"
				}
				m.Files = append(m.Files, f)
			case "symlink directory":
				if err := os.RemoveAll(filepath.Dir(name)); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(".", filepath.Dir(name)); err != nil {
					t.Fatal(err)
				}
			case "missing", "symlink", "directory", "fifo":
				if err := os.Remove(name); err != nil {
					t.Fatal(err)
				}
				var err error
				switch change {
				case "symlink":
					err = os.Symlink("../tokenizer.json", name)
				case "directory":
					err = os.Mkdir(name, 0700)
				case "fifo":
					err = syscall.Mkfifo(name, 0600)
				}
				if err != nil {
					t.Fatal(err)
				}
			}
			if data, err := m.ReadVerified(dir, p); err == nil || data != nil {
				t.Fatalf("invalid file returned %d bytes, %v", len(data), err)
			}
		})
	}
}
