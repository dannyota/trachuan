//go:build integration

package fetch

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestFetch_ExpandItemBelongsToCurrentRun(t *testing.T) {
	ctx := context.Background()
	f := &fixtureServer{files: []map[string]any{}, bodies: map[string]string{}, detailBody: `{"success":true,"data":{"id":"66518","updatedDate":null,"viewCount":7}}`}
	s, env := setup(t, f)
	var err error
	env.DB, err = pgxpool.New(ctx, testdb.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer env.DB.Close()
	recipe := strings.Repeat("a", 64)
	diagram, err := os.ReadFile(filepath.Join("..", "..", "vbpl", "testdata", "diagram-66518.json"))
	if err != nil {
		t.Fatal(err)
	}
	art, err := env.Artifacts.PublishContent(ctx, "expand", recipe, func(dir string) error {
		if writeErr := os.WriteFile(filepath.Join(dir, "detail.json"), []byte(f.detailBody), 0600); writeErr != nil {
			return writeErr
		}
		return os.WriteFile(filepath.Join(dir, "diagram.json"), diagram, 0600)
	})
	if err != nil {
		t.Fatal(err)
	}
	item := stage.Item{Key: "vn:vbpl:66518"}
	var artifactID int64
	err = env.DB.QueryRow(ctx, `INSERT INTO artifact(stage,item_key,input_hash,recipe_hash,path,sha256) VALUES ('expand',$1,$2,$3,$4,$5) RETURNING id`, item.Key, art.InputHash, recipe, art.Path, art.SHA256).Scan(&artifactID)
	if err != nil {
		t.Fatal(err)
	}
	for i, state := range []string{"done", "gap", "nonseed"} {
		t.Run(state, func(t *testing.T) {
			rel := fmt.Sprintf("20990101_%d", i+1)
			for controlSample(item.Key, rel) {
				rel = fmt.Sprintf("20990101_%d", i+10)
				i += 10
			}
			run := stage.Run{Country: "vn", Rel: rel, Snapshot: stage.Snapshot{Recipes: map[string]string{"expand": recipe}}}
			if err := env.DB.QueryRow(ctx, `INSERT INTO run(country,rel,state,snapshot) VALUES ('vn',$1,'running','{}') RETURNING id`, rel).Scan(&run.ID); err != nil {
				t.Fatal(err)
			}
			if state != "nonseed" {
				var ref any
				if state == "done" {
					ref = artifactID
				}
				if _, err := env.DB.Exec(ctx, `INSERT INTO run_item(run_id,stage,item_key,state,batch,artifact_id) VALUES ($1,'expand',$2,$3,0,$4)`, run.ID, item.Key, state, ref); err != nil {
					t.Fatal(err)
				}
			}
			f.requests.Store(0)
			out, err := s.Process(ctx, env, run, item)
			want := int32(4)
			if state == "done" {
				want = 2
			}
			if err != nil || out.State != "done" || f.requests.Load() != want {
				t.Fatalf("out=%+v calls=%d want=%d err=%v", out, f.requests.Load(), want, err)
			}
		})
	}
}
