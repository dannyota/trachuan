//go:build integration

package stage

import (
	"context"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/migrate"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"danny.vn/trachuan/apps/corpus/internal/testdb"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func setup(t *testing.T) (*pgxpool.Pool, int64) {
	t.Helper()
	ctx := context.Background()
	url := testdb.New(t)
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	var id int64
	if err := pool.QueryRow(ctx, `INSERT INTO run (country, rel, state, generation, snapshot) VALUES ('vn', '20990109', 'running', 2, '{}') RETURNING id`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO run_item (run_id, stage, item_key, state, batch, generation, attempt) VALUES ($1, 'fetch', 'item', 'pending', 0, 0, 0)`, id); err != nil {
		t.Fatal(err)
	}
	return pool, id
}

func TestMigrate_UpFromEmpty(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	url := testdb.NewEmpty(t)
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	pool, id := setup(t)
	if _, err := pool.Exec(context.Background(), `UPDATE run_item SET state='unknown' WHERE run_id=$1`, id); err == nil {
		t.Fatal("unknown state accepted")
	}
}

func TestCommitItem_FailedKeepsPlannedInputHash(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool, id := setup(t)
	batchExec(t, pool, `UPDATE run_item SET input_hash='planned' WHERE run_id=$1`, id)
	c := testCommit(id)
	c.State = "failed"
	if applied, err := CommitItem(ctx, pool, c); err != nil || !applied {
		t.Fatalf("applied=%v err=%v", applied, err)
	}
	var hash *string
	if err := pool.QueryRow(ctx, `SELECT input_hash FROM run_item WHERE run_id=$1`, id).Scan(&hash); err != nil || hash == nil || *hash != "planned" {
		t.Fatalf("hash=%v err=%v", hash, err)
	}
}

func TestGapCause_RoundTrip(t *testing.T) {
	ctx := context.Background()
	pool, id := setup(t)
	c := testCommit(id)
	status := int32(503)
	c.Gaps = []Gap{{Kind: "invalid_source_data", Detail: "failure", Source: "vbpl", Origin: OriginGateway, Cause: CauseHTTP5xx, Endpoint: "lookup", HTTPStatus: &status, EvidenceRef: "responses/1.json"}}
	if applied, err := CommitItem(ctx, pool, c); err != nil || !applied {
		t.Fatalf("commit: %v %v", applied, err)
	}
	rows, err := store.New(pool).ListRunGaps(ctx, store.ListRunGapsParams{Country: "vn", Rel: "20990109"})
	if err != nil || len(rows) != 1 {
		t.Fatalf("read gaps: %+v %v", rows, err)
	}
	got := rows[0]
	if got.Source == nil || *got.Source != "vbpl" || got.Origin == nil || *got.Origin != "gateway" || got.Cause == nil || *got.Cause != "http_5xx" || got.Endpoint == nil || *got.Endpoint != "lookup" || !got.HttpStatus.Valid || got.HttpStatus.Int32 != 503 || got.EvidenceRef == nil || *got.EvidenceRef != "responses/1.json" {
		t.Fatalf("unexpected gap: %+v", got)
	}
}

func TestCommitItem_RecheckPausedSourceFailsItem(t *testing.T) {
	ctx := context.Background()
	pool, id := setup(t)
	batchExec(t, pool, `INSERT INTO run_source(run_id,source,state,pause_reason) VALUES ($1,'vbpl','source_paused','challenge')`, id)
	c := testCommit(id)
	c.Sources = []string{"chinhphu", "vbpl"}
	c.Artifact = &artifact.Result{Path: "path", SHA256: "sha", InputHash: "input"}
	c.Gaps = []Gap{{Kind: "missing", Detail: "missing"}}
	c.RunRows = func(context.Context, pgx.Tx) error { t.Error("run rows called"); return nil }
	c.ArtifactRows = func(context.Context, pgx.Tx, int64) error { t.Error("artifact rows called"); return nil }
	if applied, err := CommitItem(ctx, pool, c); err != nil || !applied {
		t.Fatalf("applied=%v err=%v", applied, err)
	}
	var state, message string
	var artifacts, gaps int
	if err := pool.QueryRow(ctx, `SELECT state,message,(SELECT count(*) FROM artifact),(SELECT count(*) FROM gap) FROM run_item WHERE run_id=$1`, id).Scan(&state, &message, &artifacts, &gaps); err != nil || state != "failed" || message != "source_paused" || artifacts != 0 || gaps != 0 {
		t.Fatalf("state=%s message=%s artifacts=%d gaps=%d err=%v", state, message, artifacts, gaps, err)
	}
}
