package artifact

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpen_CorruptDirIsMissAndRepublishes(t *testing.T) {
	s := Store{Root: t.TempDir()}
	r := publish(t, s)
	if err := os.WriteFile(filepath.Join(s.Abs(r.Path), "data"), []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := s.Open("fetch", hash("input"), hash("recipe")); err != nil || ok {
		t.Fatalf("open: found=%v err=%v", ok, err)
	}
	repaired := publish(t, s)
	if repaired.Adopted {
		t.Fatal("adopted corrupt artifact")
	}
	if _, err := Verify(s.Abs(repaired.Path)); err != nil {
		t.Fatal(err)
	}
}
