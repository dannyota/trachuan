//go:build integration

package run_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/run"
	"danny.vn/trachuan/apps/corpus/internal/sourcegate"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/store"
)

func TestRetry_AdoptsAcquisitionsWithoutDownloadsOrObservations(t *testing.T) {
	f := newAcquisitionFixture(t)
	f.plan("expand")
	f.batch(f.expand)
	f.plan("fetch")
	f.batch(f.fetch)
	before := f.provenance()
	observations := f.scalar(`SELECT count(*) FROM run_fetch_observation`)
	if n := f.scalar(`SELECT count(*) FROM run_acquisition`); n != 2 {
		t.Fatalf("acquisitions=%d, want 2", n)
	}
	for range 2 {
		failed := f.count("/doc/2")
		f.rewind()
		f.plan("fetch")
		f.batch(f.expand)
		f.batch(f.fetch)
		f.assertRequests("/doc/1", 2)
		f.assertRequests("/doc/2", failed+2)
		if n := f.scalar(`SELECT count(*) FROM run_fetch_observation`); n != observations {
			t.Fatalf("observations=%d, want %d", n, observations)
		}
		report, err := run.Status(context.Background(), f.deps(), "vn", f.run.Rel)
		if err != nil || report.AdoptedItems != 1 {
			t.Fatalf("adopted=%d, error=%v", report.AdoptedItems, err)
		}
		if after := f.provenance(); after != before {
			t.Fatalf("file provenance changed: %s -> %s", before, after)
		}
		if n := f.scalar(`SELECT count(*) FROM gap WHERE stage='fetch' AND item_key='vn:vbpl:1' AND kind='missing_attachment'`); n != 1 {
			t.Fatalf("attachment gaps=%d", n)
		}
	}
	// A discovery rewind deletes successful expand items too.
	f.exec(`INSERT INTO run_item(run_id,stage,item_key,state,batch) VALUES ($1,'discover','central','gap',0)`, f.run.ID)
	f.exec(`INSERT INTO gap(run_id,stage,item_key,kind,detail) VALUES ($1,'discover','central','source_unavailable','')`, f.run.ID)
	f.rewind()
	f.plan("expand")
	f.plan("fetch")
	f.batch(f.expand)
	f.batch(f.fetch)
	f.assertRequests("/doc/1", 2)
}

func TestRetry_SeedsOlderWorkerBeforeDeletingItems(t *testing.T) {
	f := newAcquisitionFixture(t)
	f.plan("expand")
	f.batch(f.expand)
	f.plan("fetch")
	f.batch(f.fetch)
	f.exec(`DELETE FROM run_acquisition`)
	f.rewind()
	if n := f.scalar(`SELECT count(*) FROM run_acquisition`); n != 2 {
		t.Fatalf("seeded acquisitions=%d", n)
	}
	f.plan("fetch")
	f.batch(f.expand)
	f.batch(f.fetch)
	f.assertRequests("/doc/1", 2)
	// A partially populated table is not seeded again.
	f.exec(`DELETE FROM run_acquisition WHERE stage='expand'`)
	f.rewind()
	if n := f.scalar(`SELECT count(*) FROM run_acquisition`); n != 1 {
		t.Fatalf("unexpected reseeding: %d", n)
	}
}

func TestRetry_PlannedSkipWinsOverAcquisition(t *testing.T) {
	f := newAcquisitionFixture(t)
	f.plan("expand")
	f.batch(f.expand)
	f.plan("fetch")
	f.batch(f.fetch)
	f.rewind()
	f.plan("fetch")
	f.exec(`UPDATE run_item SET message='{"skip":{"kind":"scope_removed","detail":"removed"}}' WHERE stage='fetch' AND item_key='vn:vbpl:1'`)
	f.batch(f.fetch)
	f.assertRequests("/doc/1", 2)
	if n := f.scalar(`SELECT count(*) FROM run_item WHERE stage='fetch' AND item_key='vn:vbpl:1' AND state='gap' AND artifact_id IS NULL`); n != 1 {
		t.Fatalf("skipped items=%d", n)
	}
}

func TestRetry_InvalidBundleFallsBackAndReplacesAcquisition(t *testing.T) {
	for _, mode := range []string{"deleted", "changed file", "changed manifest", "wrong identity", "wrong recipe", "wrong stage"} {
		t.Run(mode, func(t *testing.T) {
			f := newAcquisitionFixture(t)
			f.plan("expand")
			f.batch(f.expand)
			f.plan("fetch")
			f.batch(f.fetch)
			old := f.acquired("fetch", "vn:vbpl:1")
			dir := f.bundlePath()
			switch mode {
			case "deleted":
				if err := os.RemoveAll(dir); err != nil {
					t.Fatal(err)
				}
			case "changed file":
				if err := os.WriteFile(filepath.Join(dir, "detail.json"), []byte(`{}`), 0600); err != nil {
					t.Fatal(err)
				}
			case "changed manifest":
				f.exec(`UPDATE artifact SET sha256=repeat('0',64) WHERE id=$1`, old.ArtifactID)
			case "wrong identity":
				wrong, err := f.env.Artifacts.PublishContent(context.Background(), "fetch", old.RecipeHash, func(dest string) error {
					if e := os.CopyFS(dest, os.DirFS(dir)); e != nil {
						return e
					}
					if e := os.Remove(filepath.Join(dest, "MANIFEST.sha256")); e != nil {
						return e
					}
					return os.WriteFile(filepath.Join(dest, "detail.json"), []byte(`{"success":true,"data":{"id":"wrong"}}`), 0600)
				})
				if err != nil {
					t.Fatal(err)
				}
				var wrongID int64
				err = f.env.DB.QueryRow(context.Background(), `INSERT INTO artifact(stage,item_key,input_hash,recipe_hash,path,sha256) VALUES ('fetch','vn:vbpl:1',$1,$2,$3,$4) RETURNING id`, wrong.InputHash, old.RecipeHash, wrong.Path, wrong.SHA256).Scan(&wrongID)
				if err != nil {
					t.Fatal(err)
				}
				f.exec(`UPDATE run_acquisition SET artifact_id=$1 WHERE stage='fetch'`, wrongID)
			case "wrong recipe":
				f.exec(`UPDATE artifact SET recipe_hash=repeat('0',64) WHERE id=$1`, old.ArtifactID)
			case "wrong stage":
				f.exec(`UPDATE artifact SET stage='expand' WHERE id=$1`, old.ArtifactID)
			}
			f.mu.Lock()
			f.content = "replacement"
			f.mu.Unlock()
			f.rewind()
			f.plan("fetch")
			f.batch(f.fetch)
			f.assertRequests("/doc/1", 3)
			got := f.acquired("fetch", "vn:vbpl:1")
			if got.Generation != int32(f.run.Generation) || got.ArtifactID == old.ArtifactID {
				t.Fatalf("acquisition not replaced: %+v", got)
			}
		})
	}
}

func TestAcquisition_UncommittedDirectoryIsNotAdopted(t *testing.T) {
	f := newAcquisitionFixture(t)
	out, err := f.fetch.Process(sourcegate.WithRunID(context.Background(), f.run.ID), f.env, f.run, stage.Item{Key: "vn:vbpl:1"})
	if err != nil || out.Artifact == nil {
		t.Fatalf("output=%+v error=%v", out, err)
	}
	if n := f.scalar(`SELECT count(*) FROM run_acquisition`); n != 0 {
		t.Fatalf("uncommitted acquisition=%d", n)
	}
	f.run.Generation = 2
	f.exec(`UPDATE run SET generation=2`)
	f.plan("fetch")
	f.batch(f.fetch)
	f.assertRequests("/doc/1", 2)
	if n := f.scalar(`SELECT count(*) FROM run_fetch_observation WHERE item_key='vn:vbpl:1'`); n != 4 {
		t.Fatalf("observations=%d, want 4 fresh downloads", n)
	}
}

func TestAcquisition_AnotherRunReadsSource(t *testing.T) {
	f := newAcquisitionFixture(t)
	f.plan("fetch")
	f.batch(f.fetch)
	err := f.env.DB.QueryRow(context.Background(), `INSERT INTO run(country,rel,state,generation,snapshot) SELECT country,'20990111_2','running',2,snapshot FROM run WHERE id=$1 RETURNING id`, f.run.ID).Scan(&f.run.ID)
	if err != nil {
		t.Fatal(err)
	}
	f.run.Generation = 2
	f.plan("fetch")
	f.batch(f.fetch)
	f.assertRequests("/doc/1", 2)
	if _, err = store.New(f.env.DB).GetRunAcquisition(context.Background(), store.GetRunAcquisitionParams{RunID: f.run.ID, Stage: "fetch", DocumentKey: "vn:vbpl:1"}); err != nil {
		t.Fatal(err)
	}
}

func TestRetry_AdoptionRechecksSourcePause(t *testing.T) {
	f := newAcquisitionFixture(t)
	f.plan("expand")
	f.batch(f.expand)
	f.plan("fetch")
	f.batch(f.fetch)
	old := f.acquired("fetch", "vn:vbpl:1")
	f.rewind()
	f.plan("fetch")
	f.exec(`UPDATE run_source SET state='source_paused',pause_reason='challenge' WHERE run_id=$1 AND source='vbpl'`, f.run.ID)
	f.batch(f.fetch)
	f.assertRequests("/doc/1", 2)
	if n := f.scalar(`SELECT count(*) FROM run_item WHERE stage='fetch' AND item_key='vn:vbpl:1' AND state='failed' AND message='source_paused' AND artifact_id IS NULL`); n != 1 {
		t.Fatalf("paused items=%d", n)
	}
	if got := f.acquired("fetch", "vn:vbpl:1"); got.ArtifactID != old.ArtifactID || got.Generation != old.Generation {
		t.Fatalf("pause changed acquisition: %+v", got)
	}
}

func TestRetry_UnreadableStorageStopsBeforeSourceFallback(t *testing.T) {
	f := newAcquisitionFixture(t)
	f.plan("expand")
	f.batch(f.expand)
	f.plan("fetch")
	f.batch(f.fetch)
	f.rewind()
	f.plan("fetch")
	root := f.env.Artifacts.Root
	if err := os.Rename(root, root+"-saved"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root + "-saved") })
	if err := os.WriteFile(root, []byte("unreadable storage"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := stage.RunBatch(context.Background(), f.env, f.fetch, stage.BatchInput{RunID: f.run.ID, Generation: f.run.Generation, Stage: "fetch"}, 1, stage.BatchConfig{HeartbeatEvery: time.Second, MaxAttempts: 1}, func(...any) {})
	if err == nil {
		t.Fatal("unreadable storage became a source fallback")
	}
	f.assertRequests("/doc/1", 2)
	if n := f.scalar(`SELECT count(*) FROM run_item WHERE stage='fetch' AND state='pending'`); n != 2 {
		t.Fatalf("pending=%d", n)
	}
}
