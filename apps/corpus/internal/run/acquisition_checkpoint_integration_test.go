//go:build integration

package run_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/checkpoint"
	"danny.vn/trachuan/apps/corpus/internal/run"
	"danny.vn/trachuan/apps/corpus/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRecover_CheckpointOwnsAcquisitionHistory(t *testing.T) {
	f := newAcquisitionFixture(t)
	ctx := context.Background()
	dumpTool := os.Getenv("CORPUS_PG_DUMP")
	if dumpTool == "" {
		dumpTool = "pg_dump"
	}
	dumpTool, err := exec.LookPath(dumpTool)
	if err != nil {
		t.Fatal("locate pg_dump:", err)
	}
	taker := checkpoint.Taker{Config: checkpoint.Config{Dir: t.TempDir(), ArtifactsRoot: f.env.Artifacts.Root, DatabaseURL: f.dbURL, PgDump: dumpTool}}
	f.plan("expand")
	f.plan("fetch")
	before, err := taker.Take(ctx, "vn-"+f.run.Rel, 1, "discover")
	if err != nil {
		t.Fatal(err)
	}
	f.batch(f.expand)
	f.batch(f.fetch)
	after, err := taker.Take(ctx, "vn-"+f.run.Rel, 1, "fetch")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, path             string
		acquisitions, requests int
	}{{"before", before, 0, 2}, {"after", after, 2, 0}} {
		t.Run(tc.name, func(t *testing.T) {
			f.t = t
			// Restore into a new isolated workshop, retaining every artifact directory.
			url := testdb.NewEmpty(t)
			cmd := exec.CommandContext(ctx, filepath.Join(filepath.Dir(dumpTool), "pg_restore"), "--exit-on-error", "--dbname", url, filepath.Join(tc.path, "workshop.dump"))
			if e := cmd.Run(); e != nil {
				t.Fatal("restore workshop:", e)
			}
			pool, e := pgxpool.New(ctx, url)
			if e != nil {
				t.Fatal(e)
			}
			t.Cleanup(pool.Close)
			f.env.DB = pool
			f.bindSourceStages()
			f.run.Generation = 1
			if n := f.scalar(`SELECT count(*) FROM run_acquisition`); n != tc.acquisitions {
				t.Fatalf("restored acquisitions=%d, want %d", n, tc.acquisitions)
			}
			requests := f.count("/doc/1")
			if e = run.Recover(ctx, f.deps(), "vn", f.run.Rel, false); e != nil {
				t.Fatal(e)
			}
			if n := f.scalar(`SELECT count(*) FROM run_acquisition`); n != tc.acquisitions {
				t.Fatalf("recover changed acquisitions: %d", n)
			}
			if e = run.Start(ctx, f.deps(), "vn", f.run.Rel, "fetch", ""); e != nil {
				t.Fatal(e)
			}
			f.run.Generation = 3
			f.plan("expand")
			f.plan("fetch")
			f.batch(f.expand)
			f.batch(f.fetch)
			f.assertRequests("/doc/1", requests+tc.requests)
		})
	}
}
