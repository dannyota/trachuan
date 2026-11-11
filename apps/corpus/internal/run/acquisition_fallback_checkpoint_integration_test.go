//go:build integration

package run_test

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/checkpoint"
)

func TestRetry_CheckpointsCorruptAcquisitionBeforeAndAfterFallback(t *testing.T) {
	for _, mode := range []string{"before fetch", "changed content", "same content"} {
		t.Run(mode, func(t *testing.T) {
			f := newAcquisitionFixture(t)
			f.plan("expand")
			f.batch(f.expand)
			f.plan("fetch")
			f.batch(f.fetch)
			old := f.acquired("fetch", "vn:vbpl:1")
			corrupt := filepath.Join(f.bundlePath(), "detail.json")
			if err := os.WriteFile(corrupt, []byte("corrupt"), 0600); err != nil {
				t.Fatal(err)
			}
			f.rewind()
			f.plan("fetch")
			f.batch(f.expand)
			if mode == "changed content" {
				f.mu.Lock()
				f.content = "replacement"
				f.mu.Unlock()
			}
			if mode != "before fetch" {
				f.batch(f.fetch)
			}
			acquired := f.acquired("fetch", "vn:vbpl:1")
			if mode == "changed content" && acquired.ArtifactID == old.ArtifactID {
				t.Fatal("fallback did not replace the acquisition")
			}
			if mode != "changed content" && acquired.ArtifactID != old.ArtifactID {
				t.Fatal("same-hash or unattempted acquisition changed artifact row")
			}
			if n := f.scalar(`SELECT count(*) FROM artifact WHERE id=$1`, old.ArtifactID); n != 1 {
				t.Fatal("original artifact row was deleted")
			}
			dump := os.Getenv("CORPUS_PG_DUMP")
			if dump == "" {
				dump = "pg_dump"
			}
			dump, err := exec.LookPath(dump)
			if err != nil {
				t.Fatal("locate pg_dump:", err)
			}
			taker := checkpoint.Taker{Config: checkpoint.Config{Dir: t.TempDir(), ArtifactsRoot: f.env.Artifacts.Root, DatabaseURL: f.dbURL, PgDump: dump}}
			var logs bytes.Buffer
			logger := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
			defer slog.SetDefault(logger)
			stageName := "fetch"
			if mode == "before fetch" {
				stageName = "expand"
			}
			dir, err := taker.Take(context.Background(), "vn-"+f.run.Rel, f.run.Generation, stageName)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = os.Stat(filepath.Join(dir, "workshop.dump")); err != nil {
				t.Fatal(err)
			}
			if _, err = os.Stat(filepath.Join(dir, "checkpoint.json")); err != nil {
				t.Fatal(err)
			}
			if mode == "same content" {
				if sum, e := artifact.Verify(f.env.Artifacts.Abs(acquired.Path)); e != nil || sum != old.Sha256 {
					t.Fatalf("restored bundle hash=%s: %v", sum, e)
				}
				if _, e := artifact.Verify(filepath.Join(taker.Config.Dir, "artifacts", acquired.Path)); e != nil {
					t.Fatal(e)
				}
			} else {
				if !strings.Contains(logs.String(), old.Path) || !strings.Contains(logs.String(), "skipping corrupt orphan artifact") || !strings.Contains(logs.String(), "manifest does not match") {
					t.Fatalf("missing orphan path or reason: %s", logs.String())
				}
				data, e := os.ReadFile(corrupt)
				if e != nil || string(data) != "corrupt" {
					t.Fatalf("corrupt directory changed: %q: %v", data, e)
				}
				if mode == "changed content" {
					if _, e = artifact.Verify(filepath.Join(taker.Config.Dir, "artifacts", acquired.Path)); e != nil {
						t.Fatal(e)
					}
				}
			}
		})
	}
}
