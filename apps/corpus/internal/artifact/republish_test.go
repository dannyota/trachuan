package artifact

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestPublishContent_PreservesInvalidDirectoryAndRestoresSameHash(t *testing.T) {
	s := Store{Root: t.TempDir()}
	ctx := context.Background()
	first, err := s.PublishContent(ctx, "fetch", hash("recipe"), writer("original"))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(s.Abs(first.Path), "data"), []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	second, err := s.PublishContent(ctx, "fetch", hash("recipe"), writer("original"))
	if err != nil {
		t.Fatal(err)
	}
	if second.Path != first.Path || second.SHA256 != first.SHA256 || second.Adopted {
		t.Fatalf("republished=%+v, original=%+v", second, first)
	}
	if sum, e := Verify(s.Abs(second.Path)); e != nil || sum != first.SHA256 {
		t.Fatalf("restored hash=%s: %v", sum, e)
	}
	trash, err := os.ReadDir(filepath.Join(s.Root, ".trash"))
	if err != nil || len(trash) != 1 {
		t.Fatalf("preserved directories=%v: %v", trash, err)
	}
	data, err := os.ReadFile(filepath.Join(s.Root, ".trash", trash[0].Name(), "data"))
	if err != nil || string(data) != "corrupt" {
		t.Fatalf("preserved bytes=%q: %v", data, err)
	}
}
