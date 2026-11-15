//go:build integration

package stage

import (
	"context"
	"errors"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"github.com/jackc/pgx/v5"
)

func TestCommit_AcquisitionRequiresCommittedSourceRead(t *testing.T) {
	for _, mode := range []string{"done", "attachment gap", "unavailable gap", "gap", "failed", "stale generation", "stale attempt", "paused", "rows fail", "no source", "reuse", "expand"} {
		t.Run(mode, func(t *testing.T) {
			pool, id := setup(t)
			c := testCommit(id)
			c.Sources = []string{"vbpl"}
			c.Artifact = &artifact.Result{Path: "path", SHA256: "hash", InputHash: "hash"}
			c.SourceRead = true
			switch mode {
			case "attachment gap":
				c.Gaps = []Gap{{Kind: "missing_attachment", Detail: "a"}}
			case "unavailable gap":
				c.Gaps = []Gap{{Kind: "source_unavailable", Detail: "unavailable"}}
			case "gap":
				c.State = "gap"
				c.Artifact = nil
			case "failed":
				c.State = "failed"
				c.Artifact = nil
			case "stale generation":
				c.Generation = 1
			case "stale attempt":
				batchExec(t, pool, `UPDATE run_item SET generation=2,attempt=3 WHERE run_id=$1`, id)
			case "paused":
				batchExec(t, pool, `INSERT INTO run_source(run_id,source,state,pause_reason) VALUES ($1,'vbpl','source_paused','challenge')`, id)
			case "rows fail":
				c.ArtifactRows = func(context.Context, pgx.Tx, int64) error { return errors.New("rows failed") }
			case "no source":
				c.Sources = nil
			case "reuse":
				c.SourceRead = false
			case "expand":
				c.Stage = "expand"
				batchExec(t, pool, `UPDATE run_item SET stage='expand' WHERE run_id=$1`, id)
			}
			_, err := CommitItem(context.Background(), pool, c)
			if (err != nil) != (mode == "rows fail") {
				t.Fatal(err)
			}
			var n int
			if err = pool.QueryRow(context.Background(), `SELECT count(*) FROM run_acquisition`).Scan(&n); err != nil {
				t.Fatal(err)
			}
			want := 0
			if mode == "done" || mode == "attachment gap" || mode == "expand" {
				want = 1
			}
			if n != want {
				t.Fatalf("acquisitions=%d, want %d", n, want)
			}
		})
	}
}

func TestCommit_AcquisitionReplayAndPausePreserveOriginal(t *testing.T) {
	pool, id := setup(t)
	ctx := context.Background()
	c := testCommit(id)
	c.SourceRead = true
	c.Sources = []string{"vbpl"}
	c.Artifact = &artifact.Result{Path: "original", SHA256: "original", InputHash: "original"}
	if applied, err := CommitItem(ctx, pool, c); err != nil || !applied {
		t.Fatalf("first commit=%v: %v", applied, err)
	}
	var before, after string
	if err := pool.QueryRow(ctx, `SELECT to_jsonb(a)::text FROM run_acquisition a`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	batchExec(t, pool, `INSERT INTO run_source(run_id,source,state,pause_reason) VALUES ($1,'vbpl','source_paused','challenge')`, id)
	if applied, err := CommitItem(ctx, pool, c); err != nil || applied {
		t.Fatalf("replay=%v: %v", applied, err)
	}
	conflict := c
	conflict.Artifact = &artifact.Result{Path: "conflict", SHA256: "conflict", InputHash: "conflict"}
	if applied, err := CommitItem(ctx, pool, conflict); err == nil || applied {
		t.Fatalf("conflict=%v: %v", applied, err)
	}
	batchExec(t, pool, `UPDATE run SET generation=3 WHERE id=$1`, id)
	c.Generation = 3
	if applied, err := CommitItem(ctx, pool, c); err != nil || !applied {
		t.Fatalf("paused commit=%v: %v", applied, err)
	}
	if err := pool.QueryRow(ctx, `SELECT to_jsonb(a)::text FROM run_acquisition a`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatalf("acquisition changed: %s -> %s", before, after)
	}
}

func TestAcquisitionSeed_SkipsUnavailableFetch(t *testing.T) {
	for _, kind := range []string{"source_unavailable", "missing_attachment"} {
		t.Run(kind, func(t *testing.T) {
			pool, id := setup(t)
			c := testCommit(id)
			c.Artifact = &artifact.Result{Path: "path", SHA256: "hash", InputHash: "hash"}
			c.Gaps = []Gap{{Kind: kind, Detail: "fixture"}}
			if ok, err := CommitItem(context.Background(), pool, c); err != nil || !ok {
				t.Fatalf("commit=%v err=%v", ok, err)
			}
			if _, err := pool.Exec(context.Background(), `DELETE FROM run_acquisition WHERE run_id=$1`, id); err != nil {
				t.Fatal(err)
			}
			if _, err := store.New(pool).SeedRunAcquisitions(context.Background(), id); err != nil {
				t.Fatal(err)
			}
			var count int
			if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM run_acquisition WHERE run_id=$1`, id).Scan(&count); err != nil {
				t.Fatal(err)
			}
			want := 1
			if kind == "source_unavailable" {
				want = 0
			}
			if count != want {
				t.Fatalf("count=%d want=%d", count, want)
			}
		})
	}
}
