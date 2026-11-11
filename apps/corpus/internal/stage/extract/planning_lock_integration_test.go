//go:build integration

package extract

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/ocrstore"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestExtract_PlanStageFinishesWithOCRSelection(t *testing.T) {
	for _, withStore := range []bool{false, true} {
		t.Run(fmt.Sprintf("store=%t", withStore), func(t *testing.T) {
			ctx := context.Background()
			db, err := pgxpool.New(ctx, testdb.New(t))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			artifacts := artifact.Store{Root: t.TempDir()}
			extractor := New()
			if withStore {
				extractor.WithOCR(ocrstore.Open(t.TempDir(), db), nil, "")
			}
			snapshot := stage.Snapshot{SourceList: []byte(`{"vbpl":{"selection_version":0}}`), Recipes: map[string]string{"extract": extractor.Recipe(stage.Snapshot{})}}
			encoded, err := json.Marshal(snapshot)
			if err != nil {
				t.Fatal(err)
			}
			var runID int64
			if err = db.QueryRow(ctx, `INSERT INTO run(country,rel,state,snapshot) VALUES ('vn','20990101','running',$1) RETURNING id`, encoded).Scan(&runID); err != nil {
				t.Fatal(err)
			}
			html := []byte("<p>Điều 1. Phạm vi</p><p>" + strings.Repeat("nội dung ", 60) + "</p>")
			htmlHash := fmt.Sprintf("%x", sha256.Sum256(html))
			pdfHash := fmt.Sprintf("%x", sha256.Sum256([]byte("pdf")))
			fixtures := []struct {
				id, tree, files string
				transcription   bool
			}{
				{"1", `{"success":true,"data":[{"id":"a","level":"Article","title":"Điều 1. A","content":{"content":"Điều 1. A<br>Nội dung"}}]}`, `[]`, false},
				{"2", `{"success":true,"data":[]}`, fmt.Sprintf(`[{"name":"2_content.html","sha256":"%s","size":%d}]`, htmlHash, len(html)), true},
				{"3", `{"success":true,"data":[]}`, fmt.Sprintf(`[{"name":"scan.pdf","sha256":"%s","size":3}]`, pdfHash), false},
			}
			for _, fixture := range fixtures {
				art, publishErr := artifacts.PublishContent(ctx, "fetch", strings.Repeat("a", 64), func(dir string) error {
					for name, body := range map[string]string{"tree.json": fixture.tree, "detail.json": fmt.Sprintf(`{"data":{"id":"%s"}}`, fixture.id), "files.json": fixture.files} {
						if e := os.WriteFile(filepath.Join(dir, name), []byte(body), 0600); e != nil {
							return e
						}
					}
					if fixture.transcription {
						if e := os.Mkdir(filepath.Join(dir, "files"), 0700); e != nil {
							return e
						}
						return os.WriteFile(filepath.Join(dir, "files", htmlHash), html, 0600)
					}
					return nil
				})
				if publishErr != nil {
					t.Fatal(publishErr)
				}
				key := "vn:vbpl:" + fixture.id
				var artifactID int64
				if e := db.QueryRow(ctx, `INSERT INTO artifact(stage,item_key,input_hash,recipe_hash,path,sha256) VALUES ('fetch',$1,$2,$3,$4,$5) RETURNING id`, key, art.InputHash, strings.Repeat("a", 64), art.Path, art.SHA256).Scan(&artifactID); e != nil {
					t.Fatal(e)
				}
				if _, e := db.Exec(ctx, `INSERT INTO run_item(run_id,stage,item_key,state,batch,artifact_id) VALUES ($1,'fetch',$2,'done',0,$3)`, runID, key, artifactID); e != nil {
					t.Fatal(e)
				}
			}
			planCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()
			_, err = stage.PlanStage(planCtx, stage.Env{DB: db, Artifacts: artifacts}, extractor, stage.StageInput{RunID: runID, Stage: "extract"}, snapshot.Recipes)
			if err != nil {
				t.Fatalf("extract planning did not finish: %v", err)
			}
			var count int
			if err = db.QueryRow(ctx, `SELECT count(*) FROM run_item WHERE run_id=$1 AND stage='extract'`, runID).Scan(&count); err != nil || count != 3 {
				t.Fatalf("planned items=%d err=%v", count, err)
			}
		})
	}
}
