//go:build integration

package checkpoint

import (
	"context"
	"strings"
	"testing"

	"danny.vn/traquydinh/apps/corpus/internal/testdb"
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
	taker.Config.DatabaseURL = url
	rel := corruptArtifact(t, taker)
	if _, takeErr := taker.Take(ctx, "run", 1, "fetch"); takeErr != nil {
		t.Fatal(takeErr)
	}
	if _, insertErr := db.Exec(ctx, `INSERT INTO artifact (stage,item_key,input_hash,recipe_hash,path,sha256) VALUES ('fetch','item',$1,$2,$3,$4)`, strings.Repeat("a", 64), strings.Repeat("b", 64), rel, strings.Repeat("c", 64)); insertErr != nil {
		t.Fatal(insertErr)
	}
	if _, takeErr := taker.Take(ctx, "run", 2, "fetch"); takeErr == nil {
		t.Fatal("accepted referenced corrupt artifact")
	}
}
