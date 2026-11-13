//go:build integration

package extract

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestExtract_RecordedHeadingArtifactsAndReplay(t *testing.T) {
	f := recordedHeadingFixture(t)
	for _, switched := range []bool{true, false} {
		t.Run(map[bool]string{true: "switched", false: "kept"}[switched], func(t *testing.T) {
			html := f.HTML
			if !switched {
				html = strings.ReplaceAll(html, "5/12/2000", "6/12/2000")
			}
			bundle := transcriptionBundle(t, string(f.Tree), html)
			ctx := context.Background()
			db, err := pgxpool.New(ctx, testdb.New(t))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			artifacts := artifact.Store{Root: t.TempDir()}
			fetched, err := artifacts.PublishContent(ctx, "fetch", strings.Repeat("a", 64), func(dir string) error { return os.CopyFS(dir, os.DirFS(bundle)) })
			if err != nil {
				t.Fatal(err)
			}
			var runID, artifactID int64
			if err = db.QueryRow(ctx, `INSERT INTO run(country,rel,state,snapshot) VALUES ('vn','20990105','running','{}') RETURNING id`).Scan(&runID); err != nil {
				t.Fatal(err)
			}
			if err = db.QueryRow(ctx, `INSERT INTO artifact(stage,item_key,input_hash,recipe_hash,path,sha256) VALUES ('fetch','vn:vbpl:1',$1,$2,$3,$4) RETURNING id`, fetched.InputHash, strings.Repeat("a", 64), fetched.Path, fetched.SHA256).Scan(&artifactID); err != nil {
				t.Fatal(err)
			}
			if _, err = db.Exec(ctx, `INSERT INTO run_item(run_id,stage,item_key,state,batch,artifact_id) VALUES ($1,'fetch','vn:vbpl:1','done',0,$2)`, runID, artifactID); err != nil {
				t.Fatal(err)
			}
			extractor := New()
			env := stage.Env{DB: db, Artifacts: artifacts}
			run := stage.Run{ID: runID, Snapshot: stage.Snapshot{SourceList: []byte(`{"vbpl":{"selection_version":0}}`)}}
			var first string
			for range 2 {
				items, planErr := extractor.Plan(ctx, env, run)
				if planErr != nil || len(items) != 1 {
					t.Fatalf("items=%v err=%v", items, planErr)
				}
				out, processErr := extractor.Process(ctx, env, run, items[0])
				if processErr != nil || out.Artifact == nil {
					t.Fatalf("output=%+v err=%v", out, processErr)
				}
				if first != "" && first != out.Artifact.SHA256 {
					t.Fatal("replay changed the artifact")
				}
				first = out.Artifact.SHA256
				dir := artifacts.Abs(out.Artifact.Path)
				recordData, readErr := os.ReadFile(filepath.Join(dir, "text.json"))
				if switched {
					if readErr != nil {
						t.Fatal(readErr)
					}
					var record textRecord
					if err = json.Unmarshal(recordData, &record); err != nil {
						t.Fatal(err)
					}
					if record.TextSource != "html_transcription" || record.Tree == nil || !record.Tree.SetAside || record.Tree.Rules[3] != (ruleCount{2, 2}) {
						t.Fatalf("record=%+v", record)
					}
				} else if !os.IsNotExist(readErr) {
					t.Fatalf("kept tree text.json: %v", readErr)
				}
				resolution, resolveErr := extractor.Resolve(ctx, env, run, items[0], *out.Artifact)
				if resolveErr != nil {
					t.Fatal(resolveErr)
				}
				if !switched && !hasGap(resolution.Gaps, "tree_headings_partial", "1 of 2 articles heading-only; rule 3 failed") {
					t.Fatalf("gaps=%v", resolution.Gaps)
				}
			}
		})
	}
}
