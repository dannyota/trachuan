package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
)

func TestArtifact_OverLimitFails(t *testing.T) {
	dir := t.TempDir()
	file, err := os.Create(filepath.Join(dir, "oversized.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = file.Truncate(256<<20 + 1); err != nil {
		t.Fatal(err)
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
	if err = checkArtifactFiles(dir); err == nil {
		t.Fatal("expected size error")
	}
}

func TestArtifact_RefusesEscape(t *testing.T) {
	root := t.TempDir()
	path := "../outside"
	hash := "different"
	_, err := artifactDir(artifact.Store{Root: root}, InventoryItem{ItemKey: "key", Path: &path, ArtifactSHA256: &hash})
	if err == nil {
		t.Fatal("expected path error")
	}
}

func TestArtifact_RefusesSymlinkParent(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "stage")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(outside, "child"), 0700); err != nil {
		t.Fatal(err)
	}
	path := "stage/child"
	hash := "hash"
	_, err := artifactDir(artifact.Store{Root: root}, InventoryItem{ItemKey: "key", Path: &path, ArtifactSHA256: &hash})
	if err == nil || !strings.Contains(err.Error(), "outside root") {
		t.Fatalf("expected path error, got %v", err)
	}
}
