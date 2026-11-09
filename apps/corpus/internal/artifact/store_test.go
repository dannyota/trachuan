package artifact

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func hash(s string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(s))) }
func writer(content string) func(string) error {
	return func(dir string) error { return os.WriteFile(filepath.Join(dir, "data"), []byte(content), 0600) }
}
func publish(t *testing.T, s Store) Result {
	t.Helper()
	r, err := s.Publish(context.Background(), "fetch", hash("input"), hash("recipe"), writer("data"))
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func TestPublish_NewDir(t *testing.T) {
	s := Store{Root: t.TempDir()}
	r := publish(t, s)
	if r.Adopted || r.InputHash != hash("input") {
		t.Fatalf("unexpected result: %+v", r)
	}
	got, ok, err := s.Open("fetch", hash("input"), hash("recipe"))
	if err != nil || !ok || got.SHA256 != r.SHA256 {
		t.Fatalf("open: %+v %v %v", got, ok, err)
	}
	_, ok, err = s.Open("fetch", hash("absent"), hash("recipe"))
	if err != nil || ok {
		t.Fatalf("missing: %v %v", ok, err)
	}
}
func TestPublishContent_KeyedByManifestHash(t *testing.T) {
	s := Store{Root: t.TempDir()}
	a, err := s.PublishContent(context.Background(), "fetch", hash("recipe"), writer("data"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.PublishContent(context.Background(), "fetch", hash("recipe"), writer("data"))
	if err != nil {
		t.Fatal(err)
	}
	if a.InputHash != a.SHA256 || a.Path != filepath.Join("fetch", a.SHA256+"-"+hash("recipe")) || a.Adopted || !b.Adopted || a.SHA256 != b.SHA256 {
		t.Fatalf("results: %+v %+v", a, b)
	}
}
func TestPublish_AdoptsExistingValidDir(t *testing.T) {
	s := Store{Root: t.TempDir()}
	a := publish(t, s)
	b := publish(t, s)
	if !b.Adopted || a.SHA256 != b.SHA256 {
		t.Fatalf("results: %+v %+v", a, b)
	}
}
func partial(t *testing.T, s Store) string {
	t.Helper()
	p := filepath.Join(s.Root, "fetch", hash("input")+"-"+hash("recipe"))
	if err := os.MkdirAll(p, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p, "partial"), []byte("partial"), 0600); err != nil {
		t.Fatal(err)
	}
	return p
}
func TestPublish_ReplacesPartialDir(t *testing.T) {
	s := Store{Root: t.TempDir()}
	partial(t, s)
	if _, _, err := s.Open("fetch", hash("input"), hash("recipe")); err == nil {
		t.Fatal("open accepted partial")
	}
	r := publish(t, s)
	if r.Adopted {
		t.Fatal("adopted partial")
	}
	entries, err := os.ReadDir(filepath.Join(s.Root, ".trash"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("trash: %v %v", entries, err)
	}
}
func concurrent(t *testing.T, repair bool) {
	t.Helper()
	s := Store{Root: t.TempDir()}
	if repair {
		partial(t, s)
	}
	ready := make(chan struct{}, 2)
	start := make(chan struct{})
	results := make([]Result, 2)
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = s.Publish(context.Background(), "fetch", hash("input"), hash("recipe"), func(dir string) error { ready <- struct{}{}; <-start; return writer("data")(dir) })
		}(i)
	}
	<-ready
	<-ready
	close(start)
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if results[0].SHA256 != results[1].SHA256 || results[0].Adopted == results[1].Adopted {
		t.Fatalf("results: %+v", results)
	}
	if _, err := Verify(s.Abs(results[0].Path)); err != nil {
		t.Fatal(err)
	}
}
func TestPublish_ConcurrentSameKey(t *testing.T)       { concurrent(t, false) }
func TestPublish_ConcurrentPartialRepair(t *testing.T) { concurrent(t, true) }
func TestVerify_DetectsModifiedFile(t *testing.T) {
	s := Store{Root: t.TempDir()}
	r := publish(t, s)
	if err := os.WriteFile(filepath.Join(s.Abs(r.Path), "data"), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(s.Abs(r.Path)); err == nil {
		t.Fatal("accepted modified file")
	}
}
func TestVerify_RejectsUnsafePaths(t *testing.T) {
	for _, name := range []string{"../escape", "/absolute", "link"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			if name == "link" {
				if err := os.Symlink(t.TempDir(), filepath.Join(dir, "link")); err != nil {
					t.Fatal(err)
				}
			}
			manifest := strings.Repeat("0", 64) + "  " + name + "\n"
			if err := os.WriteFile(filepath.Join(dir, "MANIFEST.sha256"), []byte(manifest), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := Verify(dir); err == nil {
				t.Fatal("accepted unsafe path")
			}
		})
	}
}

func TestPublish_SyncsBeforeRename(t *testing.T) {
	root := t.TempDir()
	var synced []string
	s := Store{Root: root, sync: func(path string) error {
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		target := filepath.Join(root, "fetch", hash("input")+"-"+hash("recipe"))
		_, statErr := os.Stat(target)
		if rel == "fetch" {
			if statErr != nil {
				t.Errorf("parent synced before rename: %v", statErr)
			}
		} else if !os.IsNotExist(statErr) {
			t.Errorf("%s synced after rename: %v", rel, statErr)
		}
		synced = append(synced, rel)
		return nil
	}}
	_, err := s.Publish(context.Background(), "fetch", hash("input"), hash("recipe"), func(dir string) error {
		if err := os.Mkdir(filepath.Join(dir, "nested"), 0700); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dir, "nested", "data"), []byte("data"), 0600)
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(synced) != 6 {
		t.Fatalf("syncs: %v", synced)
	}
	temp := filepath.Dir(synced[1])
	want := []string{".", filepath.Join(temp, "MANIFEST.sha256"), filepath.Join(temp, "nested", "data"), filepath.Join(temp, "nested"), temp, "fetch"}
	for i := range want {
		if synced[i] != want[i] {
			t.Fatalf("syncs: %v, want %v", synced, want)
		}
	}
}

func TestStore_PathsRelativeToRoot(t *testing.T) {
	s := Store{Root: t.TempDir()}
	r := publish(t, s)
	want := filepath.Join("fetch", hash("input")+"-"+hash("recipe"))
	if r.Path != want {
		t.Fatalf("published path %q, want %q", r.Path, want)
	}
	opened, ok, err := s.Open("fetch", hash("input"), hash("recipe"))
	if err != nil || !ok || opened.Path != want {
		t.Fatalf("opened: %+v, %v, %v", opened, ok, err)
	}
	adopted := publish(t, s)
	if !adopted.Adopted || adopted.Path != want {
		t.Fatalf("adopted: %+v", adopted)
	}
	if _, err := Verify(s.Abs(r.Path)); err != nil {
		t.Fatal(err)
	}
}

func TestStore_AbsKeepsAbsolutePaths(t *testing.T) {
	s := Store{Root: t.TempDir()}
	path := filepath.Join(t.TempDir(), "old-artifact")
	if got := s.Abs(path); got != path {
		t.Fatalf("got %q, want %q", got, path)
	}
}
