package checkpoint

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
)

type fakeDumper struct{ fail bool }

func (d fakeDumper) Dump(_ context.Context, _, path string) error {
	if d.fail {
		return errors.New("dump failed")
	}
	return os.WriteFile(path, []byte("dump"), 0600)
}
func setup(t *testing.T) Taker {
	t.Helper()
	return Taker{Config: Config{Dir: t.TempDir(), ArtifactsRoot: t.TempDir(), Keep: 3}, Dumper: fakeDumper{}, Now: func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }}
}
func publish(t *testing.T, root, input string) string {
	t.Helper()
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(input)))
	r, err := (artifact.Store{Root: root}).Publish(context.Background(), "fetch", hash, strings.Repeat("a", 64), func(dir string) error { return os.WriteFile(filepath.Join(dir, "data"), []byte(input), 0600) })
	if err != nil {
		t.Fatal(err)
	}
	return r.Path
}
func TestTake_CopiesOnlyNewArtifacts(t *testing.T) {
	taker := setup(t)
	first := publish(t, taker.Config.ArtifactsRoot, "first")
	if _, err := taker.Take(context.Background(), "vn-20261109", 1, "fetch"); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(taker.Config.Dir, "artifacts", first)
	if _, err := artifact.Verify(dest); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(dest, "marker")
	if err := os.WriteFile(marker, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	second := publish(t, taker.Config.ArtifactsRoot, "second")
	if _, err := taker.Take(context.Background(), "vn-20261109", 2, "fetch"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatal(err)
	}
	if _, err := artifact.Verify(filepath.Join(taker.Config.Dir, "artifacts", second)); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{".tmp", ".locks", ".trash"} {
		if _, err := os.Stat(filepath.Join(taker.Config.Dir, "artifacts", name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("copied %s: %v", name, err)
		}
	}
}
func TestTake_WritesCheckpointJSONLast(t *testing.T) {
	taker := setup(t)
	taker.Dumper = fakeDumper{fail: true}
	if _, err := taker.Take(context.Background(), "vn-20261109", 1, "fetch"); err == nil {
		t.Fatal("expected dump failure")
	}
	path := filepath.Join(taker.Config.Dir, "vn-20261109", "1-fetch", "checkpoint.json")
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("checkpoint exists: %v", err)
	}
	taker.Dumper = fakeDumper{}
	if _, err := taker.Take(context.Background(), "vn-20261109", 1, "fetch"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var metadata struct {
		Stage      string    `json:"stage"`
		Generation int       `json:"generation"`
		Time       time.Time `json:"time"`
		DumpSHA256 string    `json:"dump_sha256"`
	}
	if err := json.Unmarshal(data, &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata.Stage != "fetch" || metadata.Generation != 1 || !metadata.Time.Equal(taker.Now()) || metadata.DumpSHA256 != "b6ca0868bca6a2926b70aa1a71592038d9030fe26d4214edcfbd6cf41f2f4654" {
		t.Fatalf("metadata: %+v", metadata)
	}
}
func TestTake_KeepsLastThree(t *testing.T) {
	taker := setup(t)
	for i := 1; i <= 4; i++ {
		taker.Now = func() time.Time { return time.Date(2026, 1, i, 0, 0, 0, 0, time.UTC) }
		if _, err := taker.Take(context.Background(), "vn-20261109", i, "fetch"); err != nil {
			t.Fatal(err)
		}
	}
	for i := 1; i <= 4; i++ {
		_, err := os.Stat(filepath.Join(taker.Config.Dir, "vn-20261109", fmt.Sprintf("%d-fetch", i)))
		if (i == 1 && !errors.Is(err, os.ErrNotExist)) || (i > 1 && err != nil) {
			t.Fatalf("generation %d: %v", i, err)
		}
	}
}
func TestPgDumpVersion_RequiresMajor18(t *testing.T) {
	for _, tc := range []struct {
		version string
		wantErr bool
	}{{"pg_dump (PostgreSQL) 17.11", true}, {"pg_dump (PostgreSQL) 18.6", false}, {"invalid", true}} {
		t.Run(tc.version, func(t *testing.T) {
			err := checkVersion(context.Background(), "pg_dump", func(context.Context, string) (string, error) { return tc.version, nil })
			if (err != nil) != tc.wantErr {
				t.Fatalf("version: %v", err)
			}
		})
	}
}
