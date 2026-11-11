//go:build integration

package checkpoint

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestTake_ChecksWorkshopArtifactReferences(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	url := testdb.New(t)
	db, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	taker := setup(t)
	taker.beginSnapshot = nil
	taker.Config.DatabaseURL = url
	rel := corruptArtifact(t, taker)
	if _, takeErr := taker.Take(ctx, "run", 1, "fetch"); takeErr != nil {
		t.Fatal(takeErr)
	}
	if _, insertErr := db.Exec(ctx, `INSERT INTO artifact (stage,item_key,input_hash,recipe_hash,path,sha256) VALUES ('fetch','item',$1,$2,$3,$4)`, strings.Repeat("a", 64), strings.Repeat("b", 64), rel, strings.Repeat("c", 64)); insertErr != nil {
		t.Fatal(insertErr)
	}
	if _, takeErr := taker.Take(ctx, "run", 2, "fetch"); takeErr != nil {
		t.Fatalf("historical artifact row blocked checkpoint: %v", takeErr)
	}
	for _, sql := range []string{
		`INSERT INTO run(country,rel,state,generation,snapshot) VALUES ('vn','20990111_1','running',2,'{}')`,
		`INSERT INTO run_acquisition(run_id,stage,document_key,artifact_id,generation) SELECT r.id,'fetch','item',a.id,1 FROM run r CROSS JOIN artifact a`,
	} {
		if _, err = db.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	if _, takeErr := taker.Take(ctx, "run", 3, "expand"); takeErr != nil {
		t.Fatalf("acquisition hint blocked checkpoint: %v", takeErr)
	}
	if _, err = db.Exec(ctx, `INSERT INTO run_item(run_id,stage,item_key,state,batch,generation,artifact_id) SELECT r.id,'fetch','item','done',0,1,a.id FROM run r CROSS JOIN artifact a`); err != nil {
		t.Fatal(err)
	}
	if _, takeErr := taker.Take(ctx, "run", 4, "fetch"); takeErr == nil {
		t.Fatal("accepted corrupt artifact referenced by a current item from an older generation")
	}
	if _, err = db.Exec(ctx, `DELETE FROM run_item`); err != nil {
		t.Fatal(err)
	}
	if _, takeErr := taker.Take(ctx, "run", 5, "fetch"); takeErr != nil {
		t.Fatalf("detached acquisition blocked checkpoint: %v", takeErr)
	}
	// Older rows can carry absolute artifact paths.
	if _, err = db.Exec(ctx, `UPDATE artifact SET path=$1`, filepath.Join(taker.Config.ArtifactsRoot, rel)); err != nil {
		t.Fatal(err)
	}
	if _, takeErr := taker.Take(ctx, "run", 6, "fetch"); takeErr != nil {
		t.Fatalf("absolute acquisition path blocked checkpoint: %v", takeErr)
	}
	if _, err = db.Exec(ctx, `INSERT INTO run_item(run_id,stage,item_key,state,batch,artifact_id) SELECT r.id,'fetch','item','done',0,a.id FROM run r CROSS JOIN artifact a`); err != nil {
		t.Fatal(err)
	}
	if _, takeErr := taker.Take(ctx, "run", 7, "fetch"); takeErr == nil {
		t.Fatal("accepted current item with corrupt absolute artifact path")
	}
}
