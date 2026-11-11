package cli

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/ocrstore"
	"github.com/jackc/pgx/v5/pgtype"
)

type importStore struct{ seen map[string]bool }

func (s *importStore) Lookup(_ context.Context, hashes, _ []string, _ string) (map[string]ocrstore.Entry, error) {
	result := make(map[string]ocrstore.Entry)
	for _, hash := range hashes {
		if s.seen[hash] {
			result[hash] = ocrstore.Entry{}
		}
	}
	return result, nil
}

func (s *importStore) Put(_ context.Context, hash, engine, origin string, pages pgtype.Int4, data []byte) (bool, error) {
	if engine != "vision-ocr/builtin-latest" || origin != "banhmi" || pages.Valid || len(data) == 0 {
		panic("invalid import arguments")
	}
	present := s.seen[hash]
	s.seen[hash] = true
	return present, nil
}

func TestImport_OnlyMatchingHashes(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "ocr"), 0700); err != nil {
		t.Fatal(err)
	}
	hash := func(s string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(s))) }
	matching, present, invalid, other, existingInvalid := hash("pdf1"), hash("pdf2"), hash("pdf3"), hash("other"), hash("existing invalid")
	for name, body := range map[string][]byte{matching + ".txt": []byte("text"), present + ".txt": []byte("existing"), invalid + ".txt": {0xff}, other + ".txt": []byte("text"), "bad.txt": []byte("text"), existingInvalid + ".txt": {0xff}} {
		if err := os.WriteFile(filepath.Join(dir, "ocr", name), body, 0600); err != nil {
			t.Fatal(err)
		}
	}
	s := &importStore{seen: map[string]bool{present: true, existingInvalid: true}}
	counts, err := importBanhmi(context.Background(), s, dir, map[string]bool{matching: true, present: true, invalid: true, existingInvalid: true})
	if err != nil {
		t.Fatal(err)
	}
	if counts != (importCounts{imported: 1, present: 2, notInRun: 1, rejected: 2}) {
		t.Fatalf("counts=%+v", counts)
	}
	if s.seen[other] || s.seen[invalid] {
		t.Fatal("imported a nonmatching or invalid file")
	}
}

func TestImport_OpenPinnedDirectoryRejectsUnsafeFiles(t *testing.T) {
	root := t.TempDir()
	dir, err := os.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dir.Close() }()
	if err = os.Symlink("target", filepath.Join(root, "symlink")); err != nil {
		t.Fatal(err)
	}
	if err = syscall.Mkfifo(filepath.Join(root, "fifo"), 0600); err != nil {
		t.Fatal(err)
	}
	large, err := os.Create(filepath.Join(root, "large"))
	if err != nil {
		t.Fatal(err)
	}
	if err = large.Truncate(importTextLimit + 1); err != nil {
		t.Fatal(err)
	}
	if err = large.Close(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"symlink", "fifo", "large"} {
		t.Run(name, func(t *testing.T) {
			f, err := openImportAt(dir, name)
			if f != nil {
				_ = f.Close()
			}
			if err == nil {
				t.Fatal("accepted unsafe file")
			}
		})
	}
}
