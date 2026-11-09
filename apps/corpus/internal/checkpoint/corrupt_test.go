package checkpoint

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTake_SkipsNonArtifactEntries(t *testing.T) {
	taker := setup(t)
	for _, name := range []string{".DS_Store", "fetch/.DS_Store", "fetch/invalid", ".tmp/" + strings.Repeat("a", 64) + "-" + strings.Repeat("b", 64)} {
		path := filepath.Join(taker.Config.ArtifactsRoot, name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("stray"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := taker.Take(context.Background(), "run", 1, "fetch"); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(taker.Config.Dir, "artifacts"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("entries: %v, %v", entries, err)
	}
}

func corruptArtifact(t *testing.T, taker Taker) string {
	t.Helper()
	rel := publish(t, taker.Config.ArtifactsRoot, "corrupt")
	if err := os.WriteFile(filepath.Join(taker.Config.ArtifactsRoot, rel, "data"), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	return rel
}
func TestTake_LogsUnreferencedCorruptOrphan(t *testing.T) {
	taker := setup(t)
	rel := corruptArtifact(t, taker)
	taker.referenced = func(context.Context, string, string) (bool, error) { return false, nil }
	var logs bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	defer slog.SetDefault(old)
	if _, err := taker.Take(context.Background(), "run", 1, "fetch"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(logs.String(), rel) || !strings.Contains(logs.String(), "WARN") {
		t.Fatalf("log: %s", logs.String())
	}
	if _, err := os.Stat(filepath.Join(taker.Config.Dir, "artifacts", rel)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("copied orphan: %v", err)
	}
}
func TestTake_FailsOnReferencedCorruptArtifact(t *testing.T) {
	taker := setup(t)
	rel := corruptArtifact(t, taker)
	taker.referenced = func(_ context.Context, root, path string) (bool, error) {
		return root == taker.Config.ArtifactsRoot && path == rel, nil
	}
	if _, err := taker.Take(context.Background(), "run", 1, "fetch"); err == nil {
		t.Fatal("accepted corrupt referenced artifact")
	}
	if _, err := os.Stat(filepath.Join(taker.Config.Dir, "run", "1-fetch", "checkpoint.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("marker: %v", err)
	}
}

func TestTake_DoesNotReverifyCopiedArtifacts(t *testing.T) {
	taker := setup(t)
	rel := publish(t, taker.Config.ArtifactsRoot, "copied")
	taker.referenced = func(context.Context, string, string) (bool, error) { return true, nil }
	if _, err := taker.Take(context.Background(), "run", 1, "fetch"); err != nil {
		t.Fatal(err)
	}
	// A later change to the source no longer matters once the copy exists:
	// the checkpoint must not hash every stored artifact again.
	if err := os.WriteFile(filepath.Join(taker.Config.ArtifactsRoot, rel, "data"), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := taker.Take(context.Background(), "run", 1, "extract"); err != nil {
		t.Fatalf("second checkpoint re-verified a copied artifact: %v", err)
	}
}
