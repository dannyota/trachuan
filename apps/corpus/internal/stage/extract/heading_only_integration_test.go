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
	"danny.vn/trachuan/apps/corpus/internal/stage/fetch"
	"danny.vn/trachuan/apps/corpus/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestExtract_HeadingOnlyArtifactsIntegration(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		withText     bool
	}{
		{"transcription", "html_transcription", true}, {"kept tree", "source_tree", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			db, err := pgxpool.New(ctx, testdb.New(t))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			bundle := headingBundle(t, bareTree, tc.withText)
			artifacts := artifact.Store{Root: t.TempDir()}
			fetchArt, err := artifacts.PublishContent(ctx, "fetch", strings.Repeat("a", 64), func(dir string) error {
				return filepath.Walk(bundle, func(path string, info os.FileInfo, walkErr error) error {
					if walkErr != nil {
						return walkErr
					}
					rel, relErr := filepath.Rel(bundle, path)
					if relErr != nil {
						return relErr
					}
					if rel == "." {
						return nil
					}
					target := filepath.Join(dir, rel)
					if info.IsDir() {
						return os.Mkdir(target, 0700)
					}
					data, readErr := os.ReadFile(path)
					if readErr != nil {
						return readErr
					}
					return os.WriteFile(target, data, 0600)
				})
			})
			if err != nil {
				t.Fatal(err)
			}
			var runID, artifactID int64
			if err = db.QueryRow(ctx, `INSERT INTO run(country,rel,state,snapshot) VALUES ('vn','20990105','running','{}') RETURNING id`).Scan(&runID); err != nil {
				t.Fatal(err)
			}
			if err = db.QueryRow(ctx, `INSERT INTO artifact(stage,item_key,input_hash,recipe_hash,path,sha256) VALUES ('fetch','vn:vbpl:1',$1,$2,$3,$4) RETURNING id`, fetchArt.InputHash, strings.Repeat("a", 64), fetchArt.Path, fetchArt.SHA256).Scan(&artifactID); err != nil {
				t.Fatal(err)
			}
			if _, err = db.Exec(ctx, `INSERT INTO run_item(run_id,stage,item_key,state,batch,artifact_id) VALUES ($1,'fetch','vn:vbpl:1','done',0,$2)`, runID, artifactID); err != nil {
				t.Fatal(err)
			}
			input := InputHash(fetchArt.SHA256, tc.source)
			out, err := New().Process(ctx, stage.Env{DB: db, Artifacts: artifacts}, stage.Run{ID: runID}, stage.Item{Key: "vn:vbpl:1", InputHash: input})
			if err != nil {
				t.Fatal(err)
			}
			dir := artifacts.Abs(out.Artifact.Path)
			if err = fetch.CheckExtractLineage(dir, fetchArt.SHA256, input); err != nil {
				t.Fatal(err)
			}
			_, err = os.Stat(filepath.Join(dir, "text.json"))
			if tc.withText && err != nil {
				t.Fatal(err)
			}
			if !tc.withText && !os.IsNotExist(err) {
				t.Fatalf("text.json: %v", err)
			}
			data, err := os.ReadFile(filepath.Join(dir, "report.json"))
			if err != nil {
				t.Fatal(err)
			}
			var gaps []stage.Gap
			if err = json.Unmarshal(data, &gaps); err != nil {
				t.Fatal(err)
			}
			if hasGap(gaps, "tree_headings_only", "") == tc.withText {
				t.Fatalf("gaps=%v", gaps)
			}
		})
	}
}
