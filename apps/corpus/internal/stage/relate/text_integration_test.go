//go:build integration

package relate

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

func TestRelate_ProcessTextArtifact(t *testing.T) {
	for _, tc := range []struct {
		name, gap               string
		noExtract, consolidated bool
		want                    int
	}{
		{name: "text and diagram", want: 2},
		{name: "incomplete", gap: "table_text", want: 1},
		{name: "diagram without extract", noExtract: true, want: 1},
		{name: "consolidated", consolidated: true, want: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			db, err := pgxpool.New(ctx, testdb.New(t))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			artifacts := artifact.Store{Root: t.TempDir()}
			var bundle struct{ Diagram, Provisions json.RawMessage }
			b, err := os.ReadFile("testdata/text.json")
			if err != nil {
				t.Fatal(err)
			}
			if err = json.Unmarshal(b, &bundle); err != nil {
				t.Fatal(err)
			}
			publish := func(name, input string, files map[string]any) artifact.Result {
				t.Helper()
				art, e := artifacts.Publish(ctx, name, input, strings.Repeat("b", 64), func(dir string) error {
					for file, value := range files {
						data, marshalErr := json.Marshal(value)
						if marshalErr != nil {
							return marshalErr
						}
						if writeErr := os.WriteFile(filepath.Join(dir, file), data, 0600); writeErr != nil {
							return writeErr
						}
					}
					return nil
				})
				if e != nil {
					t.Fatal(e)
				}
				return art
			}
			fetched := publish("fetch", strings.Repeat("a", 64), map[string]any{"detail.json": map[string]any{"data": map[string]any{"id": "1", "isConsolidatedDocument": tc.consolidated}}, "diagram.json": bundle.Diagram, "files.json": []any{}})
			var runID int64
			if err = db.QueryRow(ctx, `INSERT INTO run(country,rel,state,snapshot) VALUES('vn','20990101','running','{}') RETURNING id`).Scan(&runID); err != nil {
				t.Fatal(err)
			}
			attach := func(name string, art artifact.Result) {
				t.Helper()
				var id int64
				if e := db.QueryRow(ctx, `INSERT INTO artifact(stage,item_key,input_hash,recipe_hash,path,sha256) VALUES($1,'vn:vbpl:1',$2,$3,$4,$5) RETURNING id`, name, art.InputHash, strings.Repeat("b", 64), art.Path, art.SHA256).Scan(&id); e != nil {
					t.Fatal(e)
				}
				if _, e := db.Exec(ctx, `INSERT INTO run_item(run_id,stage,item_key,state,batch,artifact_id) VALUES($1,$2,'vn:vbpl:1','done',0,$3)`, runID, name, id); e != nil {
					t.Fatal(e)
				}
			}
			attach("fetch", fetched)
			extractHash := "-"
			if tc.noExtract {
				if _, err = db.Exec(ctx, `INSERT INTO run_item(run_id,stage,item_key,state,batch) VALUES($1,'extract','vn:vbpl:1','gap',0)`, runID); err != nil {
					t.Fatal(err)
				}
			} else {
				gaps := []stage.Gap{}
				if tc.gap != "" {
					gaps = append(gaps, stage.Gap{Kind: tc.gap})
				}
				extracted := publish("extract", fetched.SHA256, map[string]any{"provisions.json": bundle.Provisions, "report.json": gaps})
				attach("extract", extracted)
				extractHash = extracted.SHA256
			}
			run := stage.Run{ID: runID, Snapshot: stage.Snapshot{SourceList: []byte(`{"relation_codes":{"10":{"kind":"amends_supplements","label":"Amends"}},"central_issuers":{"NHNN":"NGÂN HÀNG NHÀ NƯỚC VIỆT NAM"}}`)}}
			env := stage.Env{DB: db, Artifacts: artifacts}
			out, err := New().Process(ctx, env, run, stage.Item{Key: "vn:vbpl:1", InputHash: PairInputHash(fetched.SHA256, extractHash)})
			if err != nil {
				t.Fatal(err)
			}
			if out.Artifact == nil {
				t.Fatalf("out=%+v", out)
			}
			var rows []relation
			if err = readJSON(filepath.Join(artifacts.Abs(out.Artifact.Path), "relations.json"), &rows); err != nil {
				t.Fatal(err)
			}
			var report relationReport
			if err = readJSON(filepath.Join(artifacts.Abs(out.Artifact.Path), "report.json"), &report); err != nil {
				t.Fatal(err)
			}
			if len(rows) != tc.want || report.Text == nil {
				t.Fatalf("rows=%+v report=%+v", rows, report)
			}
			if tc.want == 2 && (report.Text.Emitted["suspends"] != 1 || rows[1].TargetVBPLID != "42") {
				t.Fatalf("rows=%+v report=%+v", rows, report)
			}
			resolution, err := New().Resolve(ctx, env, run, stage.Item{}, *out.Artifact)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, gap := range resolution.Gaps {
				if gap.Kind == "relation_text_source_incomplete" {
					found = true
					if gap.Detail != tc.gap {
						t.Fatalf("gap=%+v", gap)
					}
				}
			}
			if found != (tc.gap != "") {
				t.Fatalf("gaps=%+v", resolution.Gaps)
			}
		})
	}
}
