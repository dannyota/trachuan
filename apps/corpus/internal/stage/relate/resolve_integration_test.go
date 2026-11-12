//go:build integration

package relate

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRelate_ResolveReusedArtifact(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	url := testdb.New(t)
	db, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	snapshot := stage.Snapshot{SourceList: json.RawMessage(`{}`)}
	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	runs := make([]stage.Run, 2)
	for i, rel := range []string{"20261101", "20261102"} {
		runs[i] = stage.Run{Country: "vn", Rel: rel, Snapshot: snapshot}
		if err = db.QueryRow(ctx, `INSERT INTO run (country,rel,state,snapshot) VALUES ('vn',$1,'paused',$2) RETURNING id`, rel, data).Scan(&runs[i].ID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = db.Exec(ctx, `INSERT INTO listing (run_id,query_key,item_key,payload) VALUES ($1,'fixture','vn:vbpl:42','{}')`, runs[1].ID); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	for name, content := range map[string]string{"relations.json": `[{"target_vbpl_id":"42","method":"text_rule/1","target_anchor":"dieu-8/khoan-9"}]`, "report.json": `{"unmapped":[],"text":{"emitted":{"suspends":1},"gaps":{"relation_text_grammar":1},"gap_details":[{"name":"relation_text_grammar","detail":"bad sentence","source_node_id":"a"}]}}`} {
		if err = os.WriteFile(filepath.Join(dir, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	art := artifact.Result{Path: dir}
	for i, run := range runs {
		got, resolveErr := New().Resolve(ctx, stage.Env{DB: db}, run, stage.Item{}, art)
		if resolveErr != nil {
			t.Fatal(resolveErr)
		}
		if got.Gaps[len(got.Gaps)-1] != (stage.Gap{Kind: "relation_text_grammar", Detail: "0:a:bad sentence"}) {
			t.Fatalf("text gaps: %+v", got.Gaps)
		}
		if (i == 0 && (len(got.Gaps) != 2 || got.Gaps[0] != (stage.Gap{Kind: "unknown_target", Detail: "42"}))) || (i == 1 && len(got.Gaps) != 1) {
			t.Fatalf("run %d: %+v", i, got.Gaps)
		}
	}
}
