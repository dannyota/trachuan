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
	f.exec(`INSERT INTO file_hash(sha256,md5,etag_16,etag_5) VALUES ('sha','md5','16-2','5-3')`)
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
			wantBaselines := 0
			if tc.name == "after" {
				wantBaselines = 1
			}
			if n := f.scalar(`SELECT count(*) FROM file_hash`); n != wantBaselines {
				t.Fatalf("restored file hashes=%d want=%d", n, wantBaselines)
			}
			if n := f.scalar(`SELECT count(*) FROM fetch_baseline`); n != wantBaselines {
				t.Fatalf("restored baselines=%d, want %d", n, wantBaselines)
			}
			if wantBaselines == 1 {
				if n := f.scalar(`SELECT count(*) FROM fetch_baseline b JOIN run_item i ON i.run_id=b.run_id AND i.stage='fetch' AND i.item_key=b.item_key AND i.generation=b.generation AND i.attempt=b.attempt AND i.artifact_id=b.artifact_id JOIN artifact a ON a.id=b.artifact_id WHERE b.document_key='vn:vbpl:1' AND b.fetch_recipe_hash=i.recipe_hash AND a.path<>'' AND a.sha256<>''`); n != 1 {
					t.Fatal("restored baseline lost its producing commit or artifact")
				}
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
