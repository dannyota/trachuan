package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
)

func TestCheck_BundleVerificationDefersMissingPDF(t *testing.T) {
	artifacts := artifact.Store{Root: t.TempDir()}
	hash := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	art, err := artifacts.PublishContent(context.Background(), "fetch", hash, func(dir string) error {
		for name, data := range map[string]string{"detail.json": `{"data":{"id":"1"}}`, "tree.json": `{"success":true,"data":[]}`, "files.json": `[{"name":"source.pdf","sha256":"` + hash + `","size":3}]`} {
			if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0600); err != nil {
				return err
			}
		}
		if err := os.Mkdir(filepath.Join(dir, "files"), 0700); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dir, "files", hash), []byte("PDF"), 0600)
	})
	if err != nil {
		t.Fatal(err)
	}
	dir := artifacts.Abs(art.Path)
	if err = os.Remove(filepath.Join(dir, "files", hash)); err != nil {
		t.Fatal(err)
	}
	got, err := verifyCheckBundle(dir, "vn:vbpl:1")
	if err != nil || got != art.SHA256 {
		t.Fatalf("manifest=%s error=%v", got, err)
	}
}
