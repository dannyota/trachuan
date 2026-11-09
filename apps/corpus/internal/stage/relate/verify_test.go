package relate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/traquydinh/apps/corpus/internal/artifact"
	"danny.vn/traquydinh/apps/corpus/internal/stage"
)

func TestVerifyFetch_RejectsInvalidArtifact(t *testing.T) {
	store := artifact.Store{Root: t.TempDir()}
	art, err := store.Publish(context.Background(), "fetch", strings.Repeat("a", 64), strings.Repeat("b", 64), func(dir string) error { return os.WriteFile(filepath.Join(dir, "data"), []byte("valid"), 0600) })
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, hash string
		corrupt    bool
	}{{"valid", art.SHA256, false}, {"wrong hash", strings.Repeat("c", 64), false}, {"corrupt", art.SHA256, true}} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.corrupt {
				if err := os.WriteFile(filepath.Join(store.Abs(art.Path), "data"), []byte("changed"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			err := verifyFetch(store.Abs(art.Path), tc.hash)
			var retryable *stage.RetryableError
			if tc.name == "valid" && err != nil {
				t.Fatal(err)
			}
			if tc.name != "valid" && !errors.As(err, &retryable) {
				t.Fatalf("expected retryable error: %v", err)
			}
		})
	}
}
