//go:build integration

package fetch

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
)

type replaySecondary struct {
	*Stage
	output stage.Output
}

func (s replaySecondary) Process(context.Context, stage.Env, stage.Run, stage.Item) (stage.Output, error) {
	return s.output, nil
}
func TestResolve_IdentityMismatchCommitsNoRows(t *testing.T) {
	token := "abc"
	actual, env := secondaryFixture(t, false, &token)
	out := secondaryFetch(t, actual, env)
	ctx := context.Background()
	db, err := pgxpool.New(ctx, testdb.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	env.DB = db
	var id int64
	err = db.QueryRow(ctx, `INSERT INTO run(country,rel,state,generation,snapshot) VALUES ('vn','20990102','running',2,'{}') RETURNING id`).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(ctx, `INSERT INTO run_item(run_id,stage,item_key,state,batch,generation,attempt) VALUES ($1,'fetch','vn:chinhphu:1','pending',0,0,0)`, id)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(stage.Snapshot{Recipes: map[string]string{"fetch": actual.Recipe(stage.Snapshot{})}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `UPDATE run SET snapshot=$2 WHERE id=$1`, id, raw); err != nil {
		t.Fatal(err)
	}
	result, err := stage.RunBatch(ctx, env, replaySecondary{actual, out}, stage.BatchInput{RunID: id, Generation: 2, Stage: "fetch", Batch: 0}, 1, stage.BatchConfig{HeartbeatEvery: time.Second, MaxAttempts: 1}, func(...any) {})
	if err != nil || result.Failed != 1 {
		t.Fatalf("result %+v: %v", result, err)
	}
	var docs, files int
	err = db.QueryRow(ctx, `SELECT (SELECT count(*) FROM document),(SELECT count(*) FROM fetched_file)`).Scan(&docs, &files)
	if err != nil || docs != 0 || files != 0 {
		t.Fatalf("rows %d %d: %v", docs, files, err)
	}
}

func TestFetch_AliasUsesMatchingObservation(t *testing.T) {
	s, env := secondaryAliasFixture(t)
	out := secondaryFetch(t, s, env)
	if len(out.Observations) != 3 {
		t.Fatalf("observations %+v", out.Observations)
	}
	earlier := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	later := earlier.Add(time.Hour)
	out.Observations[1].FetchedAt = &earlier
	out.Observations[2].FetchedAt = &later
	ctx := context.Background()
	db, err := pgxpool.New(ctx, testdb.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	env.DB = db
	var id int64
	err = db.QueryRow(ctx, `INSERT INTO run(country,rel,state,generation,snapshot) VALUES ('vn','20990102','running',2,'{}') RETURNING id`).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(ctx, `INSERT INTO run_item(run_id,stage,item_key,state,batch,generation,attempt) VALUES ($1,'fetch','vn:chinhphu:218029','pending',0,0,0)`, id)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := s.Rows(ctx, env, *out.Artifact)
	if err != nil {
		t.Fatal(err)
	}
	applied, err := stage.CommitItem(ctx, db, stage.Commit{RunID: id, Stage: "fetch", ItemKey: "vn:chinhphu:218029", Generation: 2, Attempt: 1, RecipeHash: s.Recipe(stage.Snapshot{}), State: "done", Artifact: out.Artifact, ArtifactRows: rows, Observations: out.Observations})
	if err != nil || !applied {
		t.Fatalf("commit %v: %v", applied, err)
	}
	var url string
	var fetched time.Time
	err = db.QueryRow(ctx, `SELECT url,fetched_at FROM fetched_file`).Scan(&url, &fetched)
	if err != nil || url != *out.Observations[1].URL || !fetched.Equal(earlier) {
		t.Fatalf("row URL %q time %v: %v", url, fetched, err)
	}
	var key, sourceID, sourceURL string
	var evidence []byte
	err = db.QueryRow(ctx, `SELECT key,source_id,source_url,identity_evidence FROM document`).Scan(&key, &sourceID, &sourceURL, &evidence)
	if err != nil || key != "vn:chinhphu:218029" || sourceID != "218029" || sourceURL == "" || !strings.Contains(string(evidence), "metadata.json") || !strings.Contains(string(evidence), "detail.html") {
		t.Fatalf("document %q %q %q %s: %v", key, sourceID, sourceURL, evidence, err)
	}
}
