package artifact

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestVerifyFiles_SelectedFiles(t *testing.T) {
	s := Store{Root: t.TempDir()}
	art, err := s.PublishContent(context.Background(), "fetch", hash("recipe"), func(dir string) error {
		for _, f := range []string{"detail.json", "diagram.json", "attachment"} {
			if e := os.WriteFile(filepath.Join(dir, f), []byte(f), 0600); e != nil {
				return e
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	dir := s.Abs(art.Path)
	if err = os.WriteFile(filepath.Join(dir, "attachment"), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := VerifyFiles(dir, "detail.json", "diagram.json")
	if err != nil || got != art.SHA256 {
		t.Fatalf("hash=%q error=%v", got, err)
	}
	if _, err = VerifyFiles(dir, "missing"); err == nil {
		t.Fatal("accepted missing manifest entry")
	}
	if err = os.WriteFile(filepath.Join(dir, "detail.json"), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = VerifyFiles(dir, "detail.json"); err == nil {
		t.Fatal("accepted modified file")
	}
}
func TestVerifyFiles_RejectsSymlinksAndUnsafeNames(t *testing.T) {
	s := Store{Root: t.TempDir()}
	art := publish(t, s)
	dir := s.Abs(art.Path)
	for _, name := range []string{"../data", "/data", "data\nfoo"} {
		if _, err := VerifyFiles(dir, name); err == nil {
			t.Fatalf("accepted %q", name)
		}
	}
	if err := os.Remove(filepath.Join(dir, "data")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "MANIFEST.sha256"), filepath.Join(dir, "data")); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyFiles(dir, "data"); err == nil {
		t.Fatal("accepted symlink")
	}
}

func TestVerifyFiles_RejectsNonRegularManifest(t *testing.T) {
	dir := t.TempDir()
	if err := os.Symlink("missing", filepath.Join(dir, "MANIFEST.sha256")); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyFiles(dir, "detail.json"); err == nil {
		t.Fatal("accepted symlink manifest")
	}
}

func TestVerifyFiles_RejectsSymlinkArtifactDir(t *testing.T) {
	s := Store{Root: t.TempDir()}
	art := publish(t, s)
	link := filepath.Join(t.TempDir(), "artifact")
	if err := os.Symlink(s.Abs(art.Path), link); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyFiles(link, "data"); err == nil {
		t.Fatal("accepted symlink artifact directory")
	}
}
