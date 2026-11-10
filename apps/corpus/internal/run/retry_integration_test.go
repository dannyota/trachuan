//go:build integration

package run

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/stage"
)

func retryRun(t *testing.T, next string) Deps {
	t.Helper()
	d := lifecycleDeps(t)
	insertLifecycleRun(t, d, "paused", time.Now(), testSnapshot(), &next)
	return d
}

func TestStart_ResetsOnlySourceGapItems(t *testing.T) {
	d := retryRun(t, "fetch")
	ctx := context.Background()
	_, err := d.DB.Exec(ctx, `INSERT INTO run_item(run_id,stage,item_key,state,batch,attempt,generation) SELECT id,'fetch',k,'done',0,3,1 FROM run CROSS JOIN unnest(ARRAY['retry','mixed','legacy','accepted','source_data','invalid_payload','mixed_not_found']) k`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = d.DB.Exec(ctx, `INSERT INTO gap(run_id,stage,item_key,kind,detail,origin,cause,accepted_at) SELECT id,'fetch',k,kind,detail,origin,cause,accepted FROM run CROSS JOIN (VALUES ('retry','source_unavailable','',NULL,NULL,NULL::timestamptz),('mixed','source_unavailable','',NULL,NULL,NULL),('mixed','invalid_source_data','legal','source_data','invalid_payload',NULL),('legacy','invalid_source_data','gateway non_json_200',NULL,NULL,NULL),('accepted','invalid_source_data','','gateway','non_json_200',now()),('source_data','invalid_source_data','','source_data','invalid_payload',NULL),('invalid_payload','invalid_source_data','','gateway','invalid_payload',NULL),('mixed_not_found','source_unavailable','',NULL,NULL,NULL),('mixed_not_found','not_found','',NULL,NULL,NULL)) v(k,kind,detail,origin,cause,accepted)`)
	if err != nil {
		t.Fatal(err)
	}
	if err = Start(ctx, d, "vn", d.TaskQueue, "", ""); err != nil {
		t.Fatal(err)
	}
	rows, err := d.DB.Query(ctx, `SELECT item_key,state,attempt,generation FROM run_item ORDER BY item_key`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var key, state string
		var attempt, generation int
		if err = rows.Scan(&key, &state, &attempt, &generation); err != nil {
			t.Fatal(err)
		}
		if key == "retry" || key == "accepted" {
			if state != "pending" || attempt != 0 || generation != 2 {
				t.Errorf("%s: %s %d %d", key, state, attempt, generation)
			}
		} else if state != "done" || attempt != 3 || generation != 1 {
			t.Errorf("%s changed: %s %d %d", key, state, attempt, generation)
		}
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = d.DB.QueryRow(ctx, `SELECT count(*) FROM gap WHERE item_key IN ('retry','accepted')`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("selected gaps=%d err=%v", count, err)
	}
	var message string
	if err = d.DB.QueryRow(ctx, `SELECT message FROM run`).Scan(&message); err != nil || message != "retry: 2 items reset in fetch, rewind to fetch" {
		t.Fatalf("message=%q err=%v", message, err)
	}
}

func TestStart_RefusesDiscoverItemWithArtifact(t *testing.T) {
	d := retryRun(t, "fetch")
	ctx := context.Background()
	_, err := d.DB.Exec(ctx, `INSERT INTO artifact(stage,item_key,input_hash,recipe_hash,path,sha256) VALUES ('discover','vbpl:central','input','recipe','path','hash')`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = d.DB.Exec(ctx, `INSERT INTO run_item(run_id,stage,item_key,state,batch,artifact_id) SELECT r.id,'discover','vbpl:central','done',0,a.id FROM run r CROSS JOIN artifact a`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = d.DB.Exec(ctx, `INSERT INTO gap(run_id,stage,item_key,kind,detail) SELECT id,'discover','vbpl:central','source_unavailable','' FROM run`)
	if err != nil {
		t.Fatal(err)
	}
	err = Start(ctx, d, "vn", d.TaskQueue, "", "")
	if err == nil || !strings.Contains(err.Error(), "discover item with an artifact cannot be retried") || !strings.Contains(err.Error(), "vbpl:central") {
		t.Fatalf("error=%v", err)
	}
	var generation, gaps int
	if err = d.DB.QueryRow(ctx, `SELECT generation,(SELECT count(*) FROM gap) FROM run`).Scan(&generation, &gaps); err != nil || generation != 1 || gaps != 1 {
		t.Fatalf("generation=%d gaps=%d err=%v", generation, gaps, err)
	}
}

func TestStart_RetriesDoneDiscoverItemWithSourceGap(t *testing.T) {
	d := lifecycleDeps(t)
	d.Stages = []string{"discover", "expand", "fetch"}
	d.RecipesFor = func(stage.Snapshot) map[string]string {
		return map[string]string{"discover": "one", "expand": "two", "fetch": "three"}
	}
	snap := stage.Snapshot{SourceList: []byte(`{}`), Stages: d.Stages, Recipes: d.RecipesFor(stage.Snapshot{})}
	next := "fetch"
	insertLifecycleRun(t, d, "paused", time.Now(), snap, &next)
	ctx := context.Background()
	statements := []string{
		`INSERT INTO artifact(stage,item_key,input_hash,recipe_hash,path,sha256) VALUES ('discover','chinhphu:keyword:law','input','recipe','path','hash')`,
		`INSERT INTO run_item(run_id,stage,item_key,state,batch,attempt,generation,input_hash,artifact_id) SELECT id,'discover','chinhphu:keyword:law','done',0,3,1,'input',(SELECT id FROM artifact) FROM run`,
		`INSERT INTO run_item(run_id,stage,item_key,state,batch) SELECT id,'discover',k,'pending',0 FROM run CROSS JOIN unnest(ARRAY['vbpl:central','vbpl:named:law']) k`,
		`INSERT INTO run_item(run_id,stage,item_key,state,batch) SELECT id,'expand','seed','done',0 FROM run`,
		`INSERT INTO gap(run_id,stage,item_key,kind,detail,origin,cause) SELECT id,'discover','chinhphu:keyword:law','source_unavailable','EOF','unknown','unknown' FROM run`,
		`INSERT INTO run_document(run_id,document_key,country,source,source_id,class,disposition) SELECT id,'vn:vbpl:1','vn','vbpl','1','primary','active' FROM run`,
		`INSERT INTO run_membership_evidence(run_id,document_key,stage,item_key,rule_kind,rule_value,response_sha256,artifact_hash,evidence_path,support_state) SELECT id,'vn:vbpl:1','discover','chinhphu:keyword:law','secondary_probe','law','hash','art','path','current' FROM run`,
		`INSERT INTO run_seed_observation(run_id,seed_key,item_key,completeness,observed_targets) SELECT id,'vn:vbpl:1','seed','incomplete','[]' FROM run`,
	}
	for _, statement := range statements {
		if _, err := d.DB.Exec(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := Start(ctx, d, "vn", d.TaskQueue, "", ""); err != nil {
		t.Fatal(err)
	}
	var state string
	var attempt, generation, pending, gaps, evidence, observations, artifacts int
	var input *string
	var artifactID *int64
	err := d.DB.QueryRow(ctx, `SELECT state,attempt,generation,input_hash,artifact_id,(SELECT count(*) FROM run_item WHERE stage='discover' AND state='pending'),(SELECT count(*) FROM gap),(SELECT count(*) FROM run_membership_evidence),(SELECT count(*) FROM run_seed_observation),(SELECT count(*) FROM artifact) FROM run_item WHERE item_key='chinhphu:keyword:law'`).Scan(&state, &attempt, &generation, &input, &artifactID, &pending, &gaps, &evidence, &observations, &artifacts)
	if err != nil || state != "pending" || attempt != 0 || generation != 2 || input != nil || artifactID != nil || pending != 3 || gaps != 0 || evidence != 0 || observations != 0 || artifacts != 1 {
		t.Fatalf("state=%s attempt=%d generation=%d input=%v artifact=%v pending=%d gaps=%d evidence=%d observations=%d artifacts=%d err=%v", state, attempt, generation, input, artifactID, pending, gaps, evidence, observations, artifacts, err)
	}
}

func TestStart_RetryCompleteRunAndFence(t *testing.T) {
	d := retryRun(t, "")
	ctx := context.Background()
	_, err := d.DB.Exec(ctx, `INSERT INTO run_item(run_id,stage,item_key,state,batch,generation) SELECT id,'fetch','item','done',0,1 FROM run`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = d.DB.Exec(ctx, `INSERT INTO gap(run_id,stage,item_key,kind,detail) SELECT id,'fetch','item','source_unavailable','' FROM run`)
	if err != nil {
		t.Fatal(err)
	}
	if err = Start(ctx, d, "vn", d.TaskQueue, "", ""); err != nil {
		t.Fatal(err)
	}
	var id int64
	var next string
	if err = d.DB.QueryRow(ctx, `SELECT id,next_stage FROM run`).Scan(&id, &next); err != nil || next != "fetch" {
		t.Fatalf("next=%q err=%v", next, err)
	}
	applied, err := stage.CommitItem(ctx, d.DB, stage.Commit{RunID: id, Stage: "fetch", ItemKey: "item", Generation: 1, Attempt: 99, State: "done", Gaps: []stage.Gap{{Kind: "old", Detail: "stale"}}})
	if err != nil || applied {
		t.Fatalf("old commit applied=%v err=%v", applied, err)
	}
	var count int
	if err = d.DB.QueryRow(ctx, `SELECT count(*) FROM gap`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("gaps=%d err=%v", count, err)
	}
}

func TestStart_RetryRebuildsDependents(t *testing.T) {
	d := lifecycleDeps(t)
	d.Stages = []string{"discover", "expand", "fetch", "build"}
	d.RecipesFor = func(stage.Snapshot) map[string]string {
		return map[string]string{"discover": "one", "expand": "two", "fetch": "three", "build": "four"}
	}
	snap := stage.Snapshot{SourceList: []byte(`{}`), Stages: d.Stages, Recipes: d.RecipesFor(stage.Snapshot{})}
	next := "build"
	insertLifecycleRun(t, d, "paused", time.Now(), snap, &next)
	ctx := context.Background()
	statements := []string{
		`INSERT INTO artifact(stage,item_key,input_hash,recipe_hash,path,sha256) VALUES ('expand','seed','input','recipe','path','hash')`,
		`INSERT INTO run_item(run_id,stage,item_key,state,batch,artifact_id) SELECT id,'expand','seed','done',0,(SELECT id FROM artifact) FROM run`,
		`INSERT INTO run_item(run_id,stage,item_key,state,batch) SELECT id,s,k,'done',0 FROM run CROSS JOIN (VALUES ('discover','central'),('fetch','doc'),('build','build')) v(s,k)`,
		`INSERT INTO gap(run_id,stage,item_key,kind,detail) SELECT id,'expand','seed','source_unavailable','' FROM run`,
		`INSERT INTO gap(run_id,stage,item_key,kind,detail) SELECT id,'fetch','doc','invalid_source_data','legal' FROM run`,
		`INSERT INTO run_document(run_id,document_key,country,source,source_id,class,disposition) SELECT id,'vn:vbpl:'||k,'vn','vbpl',k,'primary','active' FROM run CROSS JOIN unnest(ARRAY['keep','drop']) k`,
		`INSERT INTO run_membership_evidence(run_id,document_key,stage,item_key,rule_kind,rule_value,response_sha256,artifact_hash,evidence_path,support_state) SELECT id,'vn:vbpl:keep','discover','central','agency','1','hash','art','path','current' FROM run`,
		`INSERT INTO run_membership_evidence(run_id,document_key,stage,item_key,rule_kind,rule_value,response_sha256,artifact_hash,evidence_path,support_state) SELECT id,'vn:vbpl:drop','discover','central','relation','1','hash','art','path','removed' FROM run`,
		`INSERT INTO run_seed_observation(run_id,seed_key,item_key,completeness,observed_targets) SELECT id,'vn:vbpl:keep','seed','incomplete','[]' FROM run`,
		`INSERT INTO run_scope(run_id,seeds_snapshot,seeds_hash,expanded_snapshot,expanded_hash,final_snapshot,final_hash,fetch_close_input_hash) SELECT id,'s','s','e','e','f','f','input' FROM run`,
	}
	for _, sql := range statements {
		if _, err := d.DB.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	if err := Start(ctx, d, "vn", d.TaskQueue, "", ""); err != nil {
		t.Fatal(err)
	}
	var items, gaps, docs, evidence, observations, artifacts int
	var seeds, expanded, final, hash *string
	err := d.DB.QueryRow(ctx, `SELECT (SELECT count(*) FROM run_item),(SELECT count(*) FROM gap),(SELECT count(*) FROM run_document),(SELECT count(*) FROM run_membership_evidence),(SELECT count(*) FROM run_seed_observation),(SELECT count(*) FROM artifact), seeds_hash,expanded_hash,final_hash,fetch_close_input_hash FROM run_scope`).Scan(&items, &gaps, &docs, &evidence, &observations, &artifacts, &seeds, &expanded, &final, &hash)
	if err != nil || items != 2 || gaps != 0 || docs != 1 || evidence != 1 || observations != 0 || artifacts != 1 || seeds == nil || expanded != nil || final != nil || hash != nil {
		t.Fatalf("items=%d gaps=%d docs=%d evidence=%d observations=%d artifacts=%d seeds=%v expanded=%v final=%v hash=%v err=%v", items, gaps, docs, evidence, observations, artifacts, seeds, expanded, final, hash, err)
	}
	var recipes []byte
	if err = d.DB.QueryRow(ctx, `SELECT snapshot->'Recipes' FROM run`).Scan(&recipes); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(recipes), `"expand": "two"`) {
		t.Fatalf("recipes=%s", recipes)
	}
}

func TestStart_RetryRejectsUntilBeforeRewind(t *testing.T) {
	d := retryRun(t, "fetch")
	ctx := context.Background()
	if _, err := d.DB.Exec(ctx, `INSERT INTO run_item(run_id,stage,item_key,state,batch) SELECT id,'fetch','item','done',0 FROM run`); err != nil {
		t.Fatal(err)
	}
	if _, err := d.DB.Exec(ctx, `INSERT INTO gap(run_id,stage,item_key,kind,detail) SELECT id,'fetch','item','source_unavailable','' FROM run`); err != nil {
		t.Fatal(err)
	}
	if err := Start(ctx, d, "vn", d.TaskQueue, "discover", ""); err == nil {
		t.Fatal("accepted until before retry")
	}
	var generation, gaps int
	if err := d.DB.QueryRow(ctx, `SELECT generation,(SELECT count(*) FROM gap) FROM run`).Scan(&generation, &gaps); err != nil || generation != 1 || gaps != 1 {
		t.Fatalf("generation=%d gaps=%d err=%v", generation, gaps, err)
	}
}

func TestStart_RetryPreservesSourcePause(t *testing.T) {
	d := retryRun(t, "fetch")
	ctx := context.Background()
	for _, sql := range []string{
		`INSERT INTO run_item(run_id,stage,item_key,state,batch) SELECT id,'fetch','item','done',0 FROM run`,
		`INSERT INTO gap(run_id,stage,item_key,kind,detail) SELECT id,'fetch','item','source_unavailable','' FROM run`,
		`INSERT INTO run_source(run_id,source,state,pause_reason) SELECT id,'vbpl','source_paused','challenge' FROM run`,
	} {
		if _, err := d.DB.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	if err := Start(ctx, d, "vn", d.TaskQueue, "", ""); err != nil {
		t.Fatal(err)
	}
	var state string
	if err := d.DB.QueryRow(ctx, `SELECT state FROM run_source WHERE source='vbpl'`).Scan(&state); err != nil || state != "source_paused" {
		t.Fatalf("source=%q err=%v", state, err)
	}
}

func TestStart_DeletesThoseGapRows(t *testing.T) {
	d := retryRun(t, "fetch")
	ctx := context.Background()
	statements := []string{
		`INSERT INTO run_item(run_id,stage,item_key,state,batch) SELECT id,'fetch',k,'done',0 FROM run CROSS JOIN unnest(ARRAY['retry','mixed','accepted']) k`,
		`INSERT INTO gap(run_id,stage,item_key,kind,detail,accepted_at,note) SELECT id,'fetch',k,kind,detail,accepted,note FROM run CROSS JOIN (VALUES ('retry','source_unavailable','first',NULL::timestamptz,NULL::text),('retry','source_unavailable','second',now(),'accepted retry'),('mixed','source_unavailable','retry',NULL,NULL),('mixed','not_found','missing',NULL,NULL),('accepted','not_found','accepted',now(),'keep note')) v(k,kind,detail,accepted,note)`,
	}
	for _, statement := range statements {
		if _, err := d.DB.Exec(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := Start(ctx, d, "vn", d.TaskQueue, "", ""); err != nil {
		t.Fatal(err)
	}
	var selected, mixed, accepted, total int
	err := d.DB.QueryRow(ctx, `SELECT count(*) FILTER (WHERE item_key='retry'), count(*) FILTER (WHERE item_key='mixed' AND ((kind='source_unavailable' AND detail='retry') OR (kind='not_found' AND detail='missing')) AND accepted_at IS NULL), count(*) FILTER (WHERE item_key='accepted' AND kind='not_found' AND detail='accepted' AND accepted_at IS NOT NULL AND note='keep note'), count(*) FROM gap`).Scan(&selected, &mixed, &accepted, &total)
	if err != nil || selected != 0 || mixed != 2 || accepted != 1 || total != 3 {
		t.Fatalf("selected=%d mixed=%d accepted=%d total=%d err=%v", selected, mixed, accepted, total, err)
	}
}

func TestStart_FrozenRecipesUnchanged(t *testing.T) {
	d := retryRun(t, "fetch")
	ctx := context.Background()
	if _, err := d.DB.Exec(ctx, `INSERT INTO run_item(run_id,stage,item_key,state,batch) SELECT id,'fetch','item','done',0 FROM run`); err != nil {
		t.Fatal(err)
	}
	if _, err := d.DB.Exec(ctx, `INSERT INTO gap(run_id,stage,item_key,kind,detail) SELECT id,'fetch','item','source_unavailable','' FROM run`); err != nil {
		t.Fatal(err)
	}
	var beforeRecipes, beforeStages, afterRecipes, afterStages []byte
	if err := d.DB.QueryRow(ctx, `SELECT snapshot->'Recipes',snapshot->'Stages' FROM run`).Scan(&beforeRecipes, &beforeStages); err != nil {
		t.Fatal(err)
	}
	if err := Start(ctx, d, "vn", d.TaskQueue, "", ""); err != nil {
		t.Fatal(err)
	}
	if err := d.DB.QueryRow(ctx, `SELECT snapshot->'Recipes',snapshot->'Stages' FROM run`).Scan(&afterRecipes, &afterStages); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(beforeRecipes, afterRecipes) || !bytes.Equal(beforeStages, afterStages) {
		t.Fatalf("recipes: %s -> %s; stages: %s -> %s", beforeRecipes, afterRecipes, beforeStages, afterStages)
	}
}

func TestRetry_ClearsPredecessorWithSeeds(t *testing.T) {
	d := retryRun(t, "fetch")
	ctx := context.Background()
	statements := []string{
		`INSERT INTO run_item(run_id,stage,item_key,state,batch) SELECT id,'discover','central','gap',0 FROM run`,
		`INSERT INTO gap(run_id,stage,item_key,kind,detail) SELECT id,'discover','central','source_unavailable','' FROM run`,
		`INSERT INTO run_scope(run_id,seeds_snapshot,seeds_hash,predecessor_run_id,predecessor_phase,predecessor_snapshot_hash) SELECT id,'s','s',id,'final','hash' FROM run`,
	}
	for _, sql := range statements {
		if _, err := d.DB.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	if err := Start(ctx, d, "vn", d.TaskQueue, "", ""); err != nil {
		t.Fatal(err)
	}
	var id *int64
	var phase, hash, seeds *string
	if err := d.DB.QueryRow(ctx, `SELECT predecessor_run_id,predecessor_phase,predecessor_snapshot_hash,seeds_hash FROM run_scope`).Scan(&id, &phase, &hash, &seeds); err != nil || id != nil || phase != nil || hash != nil || seeds != nil {
		t.Fatalf("id=%v phase=%v hash=%v seeds=%v err=%v", id, phase, hash, seeds, err)
	}
}
