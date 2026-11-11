package artifact

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenRecorded_VerifiesWholeBundleAndRecordedHash(t *testing.T) {
	for _, mode := range []string{"valid", "missing", "bundle is file", "changed file", "changed manifest", "unreadable storage"} {
		t.Run(mode, func(t *testing.T) {
			s := Store{Root: t.TempDir()}
			recipe := strings.Repeat("a", 64)
			art, err := s.PublishContent(context.Background(), "fetch", recipe, func(dir string) error {
				return os.WriteFile(filepath.Join(dir, "attachment"), []byte("original"), 0600)
			})
			if err != nil {
				t.Fatal(err)
			}
			dir := s.Abs(art.Path)
			switch mode {
			case "missing":
				err = os.RemoveAll(dir)
			case "bundle is file":
				err = os.RemoveAll(dir)
				if err == nil {
					err = os.WriteFile(dir, []byte("invalid bundle"), 0600)
				}
			case "changed file":
				err = os.WriteFile(filepath.Join(dir, "attachment"), []byte("changed"), 0600)
			case "changed manifest":
				art.SHA256 = strings.Repeat("b", 64)
			case "unreadable storage":
				err = os.RemoveAll(s.Root)
				if err == nil {
					err = os.WriteFile(s.Root, []byte("not a directory"), 0600)
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			opener, ok := any(s).(interface {
				OpenRecorded(string, string, string, string) (Result, bool, error)
			})
			if !ok {
				t.Fatal("recorded artifact verification missing")
			}
			got, found, err := opener.OpenRecorded("fetch", art.InputHash, recipe, art.SHA256)
			if mode == "unreadable storage" {
				if err == nil {
					t.Fatal("unreadable storage became a cache miss")
				}
				return
			}
			if err != nil || found != (mode == "valid") {
				t.Fatalf("found=%v error=%v", found, err)
			}
			if found && (got.SHA256 != art.SHA256 || got.InputHash != art.InputHash) {
				t.Fatalf("artifact=%+v", got)
			}
		})
	}
}
