//go:build integration

package classify

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/batchapi"
	"github.com/anthropics/anthropic-sdk-go/v2"
)

func TestArticles_DatabaseSettlementAndReplay(t *testing.T) {
	db, runID, rel := submissionDatabase(t)
	ctx := context.Background()
	r, _, client, o, families := runArticlesFixture(t)
	o.RunID, o.Release = runID, rel
	r.Repository = &PGRepository{Pool: db}
	client.beforeCreate = func(requests []batchapi.Request) {
		var id, state string
		var reserve int64
		if err := db.QueryRow(ctx, `SELECT id,state,reserved_micro_usd FROM submission WHERE run_id=$1`, runID).Scan(&id, &state, &reserve); err != nil {
			t.Fatal(err)
		}
		if state != "preparing" || reserve != 2053 || requests[0].CustomID != customID(id, 0) {
			t.Fatal("article batch not reserved before create")
		}
		if _, err := os.Stat(filepath.Join(r.IntentDir, "submissions", id+".json")); err != nil {
			t.Fatal(err)
		}
		var message anthropic.Message
		if err := json.Unmarshal([]byte(`{"stop_reason":"end_turn","content":[{"type":"text","text":"{\"dieu-1\":{\"codes\":[{\"code\":\"1.1.c1\",\"confidence\":0.9}]}}"}],"usage":{"input_tokens":100,"output_tokens":10}}`), &message); err != nil {
			t.Fatal(err)
		}
		client.results = []batchapi.Result{{CustomID: requests[0].CustomID, Type: "succeeded", Message: message}}
	}
	if _, err := r.Articles(ctx, o, "labels", articleInputs(1, ""), families, articleNodes()); err != nil {
		t.Fatal(err)
	}
	observer := &PGRepository{Pool: db}
	unlock, err := observer.Lock(ctx)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := observer.List(ctx)
	unlock()
	if err != nil || len(rows) != 1 || !rows[0].Settled || !rows[0].Items[0].Article.Nodes[0].Active || rows[0].Items[0].ArticleMeta.LabelsHash != "labels" || rows[0].Results[rows[0].Items[0].Key].Articles["dieu-1"].Codes[0].Confidence != 0.9 {
		t.Fatalf("rows %+v error %v", rows, err)
	}
	if _, err = r.Articles(ctx, o, "labels", articleInputs(1, ""), families, articleNodes()); err != nil {
		t.Fatal(err)
	}
	if client.creates != 1 || client.downloads != 1 {
		t.Fatal("replay resubmitted article batch")
	}
}
