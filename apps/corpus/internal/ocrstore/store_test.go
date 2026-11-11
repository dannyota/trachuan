package ocrstore

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/store"
)

func TestStore_ReadTextVerifiesHash(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "text"), 0700); err != nil {
		t.Fatal(err)
	}
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte("original")))
	if err := os.WriteFile(filepath.Join(dir, "text", hash+".txt"), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	s := Open(dir, nil)
	if _, err := s.ReadText(store.OcrText{TextSha256: hash}); err == nil {
		t.Fatal("accepted altered text")
	}
}

func TestStore_ReadTextRejectsInvalidAndAcceptsValid(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "text"), 0700); err != nil {
		t.Fatal(err)
	}
	s := Open(dir, nil)
	for _, tc := range []struct {
		name  string
		data  []byte
		valid bool
	}{
		{"valid", []byte("hello"), true}, {"invalid UTF-8", []byte{0xff}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hash := fmt.Sprintf("%x", sha256.Sum256(tc.data))
			if err := os.WriteFile(filepath.Join(dir, "text", hash+".txt"), tc.data, 0600); err != nil {
				t.Fatal(err)
			}
			got, err := s.ReadText(store.OcrText{TextSha256: hash})
			if (err == nil) != tc.valid || (tc.valid && string(got) != string(tc.data)) {
				t.Fatalf("valid=%v err=%v", tc.valid, err)
			}
		})
	}
	if _, err := s.ReadText(store.OcrText{TextSha256: "../../bad"}); err == nil {
		t.Fatal("accepted invalid hash")
	}
}
