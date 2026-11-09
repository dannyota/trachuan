//go:build integration

package stage

import (
	"context"
	"testing"

	"github.com/dannyota/traquydinh/apps/corpus/internal/artifact"
	"github.com/dannyota/traquydinh/apps/corpus/internal/migrate"
	"github.com/dannyota/traquydinh/apps/corpus/internal/testdb"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func setup(t *testing.T) (*pgxpool.Pool, int64) {
	t.Helper()
	ctx := context.Background()
	url := testdb.New(t)
	if err := migrate.Up(ctx, url); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	var id int64
	if err := pool.QueryRow(ctx, `INSERT INTO run (country, rel, state, generation, snapshot) VALUES ('vn', '20261109', 'running', 2, '{}') RETURNING id`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO run_item (run_id, stage, item_key, state, batch, generation, attempt) VALUES ($1, 'fetch', 'item', 'pending', 0, 0, 0)`, id); err != nil {
		t.Fatal(err)
	}
	return pool, id
}

func TestMigrate_UpFromEmpty(t *testing.T) {
	ctx := context.Background()
	url := testdb.New(t)
	if err := migrate.Up(ctx, url); err != nil {
		t.Fatal(err)
	}
	if err := migrate.Up(ctx, url); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	for _, name := range []string{"run", "run_item", "artifact", "gap", "listing", "document", "fetched_file", "provision", "relation"} {
		var exists bool
		if err := pool.QueryRow(ctx, `SELECT to_regclass($1) IS NOT NULL`, name).Scan(&exists); err != nil || !exists {
			t.Fatalf("table %s: exists=%v, err=%v", name, exists, err)
		}
	}
}

func testCommit(id int64) Commit {
	return Commit{RunID: id, Stage: "fetch", ItemKey: "item", Generation: 2, Attempt: 1, RecipeHash: "recipe", State: "done"}
}

func TestCommitItem_IgnoresLowerFence(t *testing.T) {
	ctx := context.Background()
	pool, id := setup(t)
	c := testCommit(id)
	c.Message = "current"
	if applied, err := CommitItem(ctx, pool, c); err != nil || !applied {
		t.Fatalf("first commit: applied=%v err=%v", applied, err)
	}
	c.Generation, c.Attempt, c.Message = 1, 5, "stale"
	c.Artifact = &artifact.Result{Path: "stale", SHA256: "stale", InputHash: "stale"}
	c.Gaps = []Gap{{Kind: "stale", Detail: "stale"}}
	c.RunRows = func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO listing VALUES ($1, 'q', 'item', '{}')`, id)
		return err
	}
	if applied, err := CommitItem(ctx, pool, c); err != nil || applied {
		t.Fatalf("stale commit: applied=%v err=%v", applied, err)
	}
	var generation, attempt, artifacts, gaps, listings int
	var message string
	if err := pool.QueryRow(ctx, `SELECT generation, attempt, message, (SELECT count(*) FROM artifact), (SELECT count(*) FROM gap), (SELECT count(*) FROM listing) FROM run_item WHERE run_id=$1`, id).Scan(&generation, &attempt, &message, &artifacts, &gaps, &listings); err != nil {
		t.Fatal(err)
	}
	if generation != 2 || attempt != 1 || message != "current" || artifacts != 0 || gaps != 0 || listings != 0 {
		t.Fatalf("stale commit changed rows: %d %d %q %d %d %d", generation, attempt, message, artifacts, gaps, listings)
	}
}

func TestCommitItem_IgnoresLowerAttemptInSameGeneration(t *testing.T) {
	ctx := context.Background()
	pool, id := setup(t)
	c := testCommit(id)
	c.Attempt, c.Message = 3, "newer attempt"
	if applied, err := CommitItem(ctx, pool, c); err != nil || !applied {
		t.Fatalf("attempt 3: applied=%v err=%v", applied, err)
	}
	c.Attempt, c.Message, c.State = 2, "older attempt", "failed"
	if applied, err := CommitItem(ctx, pool, c); err != nil || applied {
		t.Fatalf("attempt 2 after 3: applied=%v err=%v", applied, err)
	}
	var attempt int
	var state, message string
	if err := pool.QueryRow(ctx, `SELECT attempt, state, message FROM run_item WHERE run_id=$1`, id).Scan(&attempt, &state, &message); err != nil {
		t.Fatal(err)
	}
	if attempt != 3 || state != "done" || message != "newer attempt" {
		t.Fatalf("older attempt changed the item: %d %q %q", attempt, state, message)
	}
}

func TestCommitItem_SameArtifactTwiceKeepsOneRow(t *testing.T) {
	ctx := context.Background()
	pool, id := setup(t)
	c := testCommit(id)
	c.Artifact = &artifact.Result{Path: "path", SHA256: "sha", InputHash: "input"}
	calls := 0
	c.ArtifactRows = func(context.Context, pgx.Tx, int64) error { calls++; return nil }
	for range 2 {
		if applied, err := CommitItem(ctx, pool, c); err != nil || !applied {
			t.Fatalf("commit: applied=%v err=%v", applied, err)
		}
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM artifact`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 || calls != 1 {
		t.Fatalf("artifacts=%d, callback calls=%d", count, calls)
	}
}

func TestCommitItem_RunRowsAndGapsIdempotent(t *testing.T) {
	ctx := context.Background()
	pool, id := setup(t)
	c := testCommit(id)
	c.Gaps = []Gap{{Kind: "missing", Detail: "file"}}
	c.RunRows = func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO listing VALUES ($1, 'q', 'item', '{}') ON CONFLICT (run_id, query_key, item_key) DO UPDATE SET payload=excluded.payload`, id)
		return err
	}
	for range 2 {
		if applied, err := CommitItem(ctx, pool, c); err != nil || !applied {
			t.Fatalf("commit: applied=%v err=%v", applied, err)
		}
	}
	var listings, gaps int
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM listing), (SELECT count(*) FROM gap)`).Scan(&listings, &gaps); err != nil {
		t.Fatal(err)
	}
	if listings != 1 || gaps != 1 {
		t.Fatalf("listings=%d, gaps=%d", listings, gaps)
	}
}

func TestCommitItem_GapWithoutArtifact(t *testing.T) {
	ctx := context.Background()
	pool, id := setup(t)
	c := testCommit(id)
	c.State, c.Gaps = "gap", []Gap{{Kind: "not_found", Detail: "item"}}
	if applied, err := CommitItem(ctx, pool, c); err != nil || !applied {
		t.Fatalf("commit: applied=%v err=%v", applied, err)
	}
	var state string
	var artifactID *int64
	var gaps int
	if err := pool.QueryRow(ctx, `SELECT state, artifact_id, (SELECT count(*) FROM gap WHERE run_id=$1 AND kind='not_found') FROM run_item WHERE run_id=$1`, id).Scan(&state, &artifactID, &gaps); err != nil {
		t.Fatal(err)
	}
	if state != "gap" || artifactID != nil || gaps != 1 {
		t.Fatalf("state=%q, artifact_id=%v, gaps=%d", state, artifactID, gaps)
	}
}

func TestRunItem_RejectsUnknownState(t *testing.T) {
	pool, id := setup(t)
	if _, err := pool.Exec(context.Background(), `UPDATE run_item SET state='unknown' WHERE run_id=$1`, id); err == nil {
		t.Fatal("unknown state accepted")
	}
}
