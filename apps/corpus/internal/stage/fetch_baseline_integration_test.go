//go:build integration

package stage

import (
	"context"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/store"
)

func TestCommit_FetchBaseline(t *testing.T) {
	for _, mode := range []string{"replace", "other recipe", "attachment gap", "gap", "failed", "no artifact", "chinhphu", "expand", "adopted", "replay", "stale attempt", "stale generation", "paused"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			pool, id := setup(t)
			batchExec(t, pool, `UPDATE run_item SET item_key='vn:vbpl:1' WHERE run_id=$1`, id)
			first := testCommit(id)
			first.ItemKey = "vn:vbpl:1"
			first.Artifact = &artifact.Result{Path: "first", SHA256: "first", InputHash: "first"}
			first.Sources, first.SourceRead = []string{"vbpl"}, true
			if ok, err := CommitItem(ctx, pool, first); err != nil || !ok {
				t.Fatalf("first commit: %v %v", ok, err)
			}
			got, getErr := store.New(pool).GetFetchBaseline(ctx, store.GetFetchBaselineParams{DocumentKey: "vn:vbpl:1", FetchRecipeHash: "recipe"})
			if getErr != nil || got.RunID != id || got.ItemKey != first.ItemKey || got.Generation != 2 || got.Attempt != 1 || got.Path != "first" || got.Sha256 != "first" || got.RecordedAt.IsZero() {
				t.Fatalf("first baseline: %+v: %v", got, getErr)
			}
			var before string
			if err := pool.QueryRow(ctx, `SELECT to_jsonb(b)::text FROM fetch_baseline b`).Scan(&before); err != nil {
				t.Fatal(err)
			}
			var secondID int64
			if err := pool.QueryRow(ctx, `INSERT INTO run(country,rel,state,generation,snapshot) VALUES ('vn','20990110','running',2,'{}') RETURNING id`).Scan(&secondID); err != nil {
				t.Fatal(err)
			}
			batchExec(t, pool, `INSERT INTO run_item(run_id,stage,item_key,state,batch,generation,attempt) VALUES ($1,'fetch','vn:vbpl:1','pending',0,0,0)`, secondID)
			c := first
			c.RunID, c.Attempt = secondID, 2
			c.Artifact = &artifact.Result{Path: "second", SHA256: "second", InputHash: "second"}
			switch mode {
			case "other recipe":
				c.RecipeHash = "other"
			case "attachment gap":
				c.Gaps = []Gap{{Kind: "missing_attachment", Detail: "file"}}
			case "gap":
				c.State = "gap"
			case "failed":
				c.State = "failed"
			case "no artifact":
				c.Artifact = nil
			case "chinhphu":
				c.ItemKey = "vn:chinhphu:1"
				batchExec(t, pool, `UPDATE run_item SET item_key='vn:chinhphu:1' WHERE run_id=$1`, secondID)
			case "expand":
				c.Stage = "expand"
				batchExec(t, pool, `UPDATE run_item SET stage='expand' WHERE run_id=$1`, secondID)
			case "adopted":
				c.Message = AcquisitionMessage
			case "replay":
				c = first
			case "stale attempt":
				batchExec(t, pool, `UPDATE run_item SET generation=2,attempt=3 WHERE run_id=$1`, secondID)
			case "stale generation":
				c.Generation = 1
			case "paused":
				batchExec(t, pool, `INSERT INTO run_source(run_id,source,state,pause_reason) VALUES ($1,'vbpl','source_paused','challenge')`, secondID)
			}
			ok, commitErr := CommitItem(ctx, pool, c)
			if commitErr != nil || ok != (mode != "replay" && mode != "stale attempt" && mode != "stale generation") {
				t.Fatalf("second commit: %v %v", ok, commitErr)
			}
			var count int
			wantCount := 1
			if mode == "other recipe" {
				wantCount = 2
			}
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM fetch_baseline`).Scan(&count); err != nil || count != wantCount {
				t.Fatalf("baseline count=%d: %v", count, err)
			}
			var after string
			if err := pool.QueryRow(ctx, `SELECT to_jsonb(b)::text FROM fetch_baseline b WHERE fetch_recipe_hash = 'recipe'`).Scan(&after); err != nil {
				t.Fatal(err)
			}
			if mode == "other recipe" {
				other, otherErr := store.New(pool).GetFetchBaseline(ctx, store.GetFetchBaselineParams{DocumentKey: "vn:vbpl:1", FetchRecipeHash: "other"})
				if otherErr != nil || other.RunID != secondID || other.Path != "second" {
					t.Fatalf("other recipe baseline: %+v: %v", other, otherErr)
				}
			}
			if mode != "replace" && mode != "attachment gap" {
				if before != after {
					t.Fatalf("baseline changed: %s -> %s", before, after)
				}
				return
			}
			var runID, artifactID int64
			var key, recipe, item, path, sha string
			var generation, attempt int
			queryErr := pool.QueryRow(ctx, `SELECT b.document_key,b.fetch_recipe_hash,b.artifact_id,b.run_id,b.item_key,b.generation,b.attempt,a.path,a.sha256 FROM fetch_baseline b JOIN artifact a ON a.id=b.artifact_id`).Scan(&key, &recipe, &artifactID, &runID, &item, &generation, &attempt, &path, &sha)
			if queryErr != nil || key != "vn:vbpl:1" || recipe != "recipe" || artifactID == 0 || runID != secondID || item != key || generation != 2 || attempt != 2 || path != "second" || sha != "second" {
				t.Fatalf("baseline: %q %q %d %d %q %d %d %q %q: %v", key, recipe, artifactID, runID, item, generation, attempt, path, sha, queryErr)
			}
		})
	}
}
