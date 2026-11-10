//go:build integration

package stage

import (
	"context"
	"testing"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"github.com/jackc/pgx/v5"
)

func observationCommit(id int64) Commit {
	c := testCommit(id)
	c.Artifact = &artifact.Result{Path: "p", SHA256: "sha", InputHash: "input"}
	stamp := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	name := "a"
	c.Observations = []FetchObservation{{ResourceKey: `["attachment","a",null]`, Source: "vbpl", Name: &name, Path: "files/hash", ContentSHA256: "hash", Size: 1, Acquisition: "network", FetchedAt: &stamp}}
	return c
}

func TestCommit_SameAttemptIdenticalIsNoop(t *testing.T) {
	for _, state := range []string{"done", "gap", "failed"} {
		t.Run(state, func(t *testing.T) {
			ctx := context.Background()
			pool, id := setup(t)
			c := observationCommit(id)
			c.State = state
			if ok, err := CommitItem(ctx, pool, c); err != nil || !ok {
				t.Fatalf("first: %v %v", ok, err)
			}
			batchExec(t, pool, `INSERT INTO run_source(run_id,source,state,pause_reason) VALUES ($1,'vbpl','source_paused','challenge')`, id)
			c.Sources = []string{"vbpl"}
			c.RunRows = func(context.Context, pgx.Tx) error { t.Error("replay wrote rows"); return nil }
			if ok, err := CommitItem(ctx, pool, c); err != nil || ok {
				t.Fatalf("replay: %v %v", ok, err)
			}
		})
	}
}

func TestCommit_SameAttemptConflictRollsBack(t *testing.T) {
	for _, change := range []string{"hash", "time", "state", "message", "observation"} {
		t.Run(change, func(t *testing.T) {
			ctx := context.Background()
			pool, id := setup(t)
			c := observationCommit(id)
			if ok, err := CommitItem(ctx, pool, c); err != nil || !ok {
				t.Fatalf("first: %v %v", ok, err)
			}
			batchExec(t, pool, `INSERT INTO run_source(run_id,source,state,pause_reason) VALUES ($1,'vbpl','source_paused','challenge')`, id)
			c.Sources = []string{"vbpl"}
			switch change {
			case "hash":
				c.Artifact = &artifact.Result{Path: "x", SHA256: "different", InputHash: "other"}
			case "time":
				later := c.Observations[0].FetchedAt.Add(time.Hour)
				c.Observations[0].FetchedAt = &later
			case "state":
				c.State = "failed"
			case "message":
				c.Message = "different"
			case "observation":
				c.Observations = nil
			}
			if ok, _ := CommitItem(ctx, pool, c); ok {
				t.Fatal("conflict applied")
			}
			var state string
			var count int
			if err := pool.QueryRow(ctx, `SELECT state,(SELECT count(*) FROM run_fetch_observation WHERE run_id=$1) FROM run_item WHERE run_id=$1`, id).Scan(&state, &count); err != nil || state != "done" || count != 1 {
				t.Fatalf("state %s count %d: %v", state, count, err)
			}
		})
	}
}

func TestCommit_ObservationsOnlyWithNewResult(t *testing.T) {
	ctx := context.Background()
	pool, id := setup(t)
	c := observationCommit(id)
	batchExec(t, pool, `INSERT INTO run_source(run_id,source,state,pause_reason) VALUES ($1,'vbpl','source_paused','challenge')`, id)
	c.Sources = []string{"vbpl"}
	if ok, err := CommitItem(ctx, pool, c); err != nil || !ok {
		t.Fatalf("commit: %v %v", ok, err)
	}
	var state string
	var count int
	if err := pool.QueryRow(ctx, `SELECT state,(SELECT count(*) FROM run_fetch_observation WHERE run_id=$1) FROM run_item WHERE run_id=$1`, id).Scan(&state, &count); err != nil || state != "failed" || count != 0 {
		t.Fatalf("state %s count %d: %v", state, count, err)
	}
}

func TestCommit_FetchedAtFromFirstObservation(t *testing.T) {
	ctx := context.Background()
	pool, id := setup(t)
	c := observationCommit(id)
	c.ArtifactRows = func(ctx context.Context, tx pgx.Tx, id int64) error {
		if _, err := tx.Exec(ctx, `INSERT INTO document (artifact_id,key,country,source,source_id,vbpl_id,number,title,doc_type,issuer,issue_date,eff_from,eff_to,status_code,status_name,is_consolidated,has_content) VALUES ($1,'vn:vbpl:1','vn','vbpl','1','1','','','','','','','','','',false,false)`, id); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO fetched_file (artifact_id,name,sha256,size,path) VALUES ($1,'a','hash',1,'files/hash'),($1,'b','hash',1,'files/hash')`, id)
		return err
	}
	if ok, err := CommitItem(ctx, pool, c); err != nil || !ok {
		t.Fatalf("first: %v %v", ok, err)
	}
	c.Attempt++
	later := c.Observations[0].FetchedAt.Add(time.Hour)
	c.Observations[0].FetchedAt = &later
	if ok, err := CommitItem(ctx, pool, c); err != nil || !ok {
		t.Fatalf("reuse: %v %v", ok, err)
	}
	var got time.Time
	if err := pool.QueryRow(ctx, `SELECT fetched_at FROM fetched_file WHERE name='a'`).Scan(&got); err != nil || !got.Equal(later.Add(-time.Hour)) {
		t.Fatalf("time %v: %v", got, err)
	}
	var unmatched *time.Time
	if err := pool.QueryRow(ctx, `SELECT fetched_at FROM fetched_file WHERE name='b'`).Scan(&unmatched); err != nil || unmatched != nil {
		t.Fatalf("unmatched time %v: %v", unmatched, err)
	}
}

func TestCommit_StaleFencePreservesObservations(t *testing.T) {
	for _, stale := range []string{"generation", "attempt"} {
		t.Run(stale, func(t *testing.T) {
			ctx := context.Background()
			pool, id := setup(t)
			c := observationCommit(id)
			c.Attempt = 3
			if ok, err := CommitItem(ctx, pool, c); err != nil || !ok {
				t.Fatalf("first: %v %v", ok, err)
			}
			if stale == "generation" {
				c.Generation = 1
				c.Attempt = 9
			} else {
				c.Attempt = 2
			}
			c.Artifact = &artifact.Result{Path: "new", SHA256: "different", InputHash: "different"}
			if ok, err := CommitItem(ctx, pool, c); err != nil || ok {
				t.Fatalf("stale: %v %v", ok, err)
			}
			var hash string
			var n int
			if err := pool.QueryRow(ctx, `SELECT a.sha256,(SELECT count(*) FROM run_fetch_observation WHERE run_id=$1) FROM run_item i JOIN artifact a ON a.id=i.artifact_id WHERE i.run_id=$1`, id).Scan(&hash, &n); err != nil || hash != "sha" || n != 1 {
				t.Fatalf("hash=%s observations=%d err=%v", hash, n, err)
			}
		})
	}
}

func TestCommit_NoArtifactWritesNoObservations(t *testing.T) {
	ctx := context.Background()
	pool, id := setup(t)
	c := observationCommit(id)
	c.Artifact = nil
	c.State = "failed"
	if ok, err := CommitItem(ctx, pool, c); err != nil || !ok {
		t.Fatalf("commit: %v %v", ok, err)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM run_fetch_observation`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("count %d: %v", count, err)
	}
}

func TestCommit_ReusedArtifactKeepsUnknownTime(t *testing.T) {
	ctx := context.Background()
	pool, id := setup(t)
	c := observationCommit(id)
	c.Observations[0].FetchedAt = nil
	c.Observations[0].Acquisition = "cache"
	c.ArtifactRows = func(ctx context.Context, tx pgx.Tx, id int64) error {
		if _, err := tx.Exec(ctx, `INSERT INTO document (artifact_id,key,country,source,source_id,vbpl_id,number,title,doc_type,issuer,issue_date,eff_from,eff_to,status_code,status_name,is_consolidated,has_content) VALUES ($1,'vn:vbpl:1','vn','vbpl','1','1','','','','','','','','','',false,false)`, id); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO fetched_file (artifact_id,name,sha256,size,path) VALUES ($1,'a','hash',1,'files/hash')`, id)
		return err
	}
	if ok, err := CommitItem(ctx, pool, c); err != nil || !ok {
		t.Fatalf("first: %v %v", ok, err)
	}
	c.Attempt++
	stamp := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	c.Observations[0].FetchedAt = &stamp
	c.Observations[0].Acquisition = "network"
	if ok, err := CommitItem(ctx, pool, c); err != nil || !ok {
		t.Fatalf("reuse: %v %v", ok, err)
	}
	var got *time.Time
	if err := pool.QueryRow(ctx, `SELECT fetched_at FROM fetched_file`).Scan(&got); err != nil || got != nil {
		t.Fatalf("time %v: %v", got, err)
	}
}

func TestCommit_NewAttemptAppendsObservations(t *testing.T) {
	ctx := context.Background()
	pool, id := setup(t)
	c := observationCommit(id)
	for attempt := 1; attempt <= 2; attempt++ {
		c.Attempt = attempt
		if ok, err := CommitItem(ctx, pool, c); err != nil || !ok {
			t.Fatalf("attempt %d: %v %v", attempt, ok, err)
		}
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM run_fetch_observation WHERE run_id=$1`, id).Scan(&count); err != nil || count != 2 {
		t.Fatalf("count %d: %v", count, err)
	}
}

func TestCommit_TerminalReplayWithoutArtifact(t *testing.T) {
	for _, state := range []string{"done", "gap", "failed"} {
		t.Run(state, func(t *testing.T) {
			ctx := context.Background()
			pool, id := setup(t)
			c := testCommit(id)
			c.State = state
			c.Message = "original"
			if ok, err := CommitItem(ctx, pool, c); err != nil || !ok {
				t.Fatalf("commit: %v %v", ok, err)
			}
			batchExec(t, pool, `INSERT INTO run_source(run_id,source,state,pause_reason) VALUES ($1,'vbpl','source_paused','challenge')`, id)
			c.Sources = []string{"vbpl"}
			if ok, err := CommitItem(ctx, pool, c); err != nil || ok {
				t.Fatalf("replay: %v %v", ok, err)
			}
			c.Message = "changed"
			if ok, err := CommitItem(ctx, pool, c); err == nil || ok {
				t.Fatalf("conflict: %v %v", ok, err)
			}
		})
	}
}
