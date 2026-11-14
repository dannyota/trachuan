//go:build integration

package cli

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/testdb"
	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestReleaseRow_ReadOnlyMetadataAndAcceptedGaps(t *testing.T) {
	ctx := context.Background()
	admin, base, err := testpg.OpenAdministrator(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = admin.Close(ctx) }()
	name, key, err := testpg.CreateCorpus(ctx, admin, t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer testpg.DropCorpusDatabase(t, admin, name, key)
	base.Path = "/" + name
	builderURL := base.String()
	corpus, err := pgxpool.New(ctx, builderURL)
	if err != nil {
		t.Fatal("open synthetic corpus")
	}
	defer corpus.Close()
	rel := strings.TrimPrefix(key, "vn/")
	hash := strings.Repeat("a", 64)
	_, err = corpus.Exec(ctx, `CREATE EXTENSION vector; CREATE EXTENSION ltree;
	CREATE TABLE release (singleton boolean, country text, rel text, schema_version int,
	dense_recipe jsonb, chunk_recipe jsonb, lexical_recipe jsonb, retrieval_recipe jsonb,
	taxonomy_version text, source_manifest_hash text, build_input_hash text,
	build_recipe_hash text, evaluation_summary jsonb, lexical_stats jsonb, built_at timestamptz)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = corpus.Exec(ctx, `INSERT INTO release VALUES (true,'vn',$1,6,
	'{"document":{},"query":{}}','{"version":1}','{}','{"version":3}',
	'',$2,$2,$2,'{"status":"not_run","release_eligible":false}','{}','2099-01-01T00:00:00Z')`, rel, hash)
	if err != nil {
		t.Fatal(err)
	}
	workshopURL := testdb.New(t)
	workshop, err := pgxpool.New(ctx, workshopURL)
	if err != nil {
		t.Fatal("open synthetic workshop")
	}
	defer workshop.Close()
	var runID, artifactID int64
	if err = workshop.QueryRow(ctx, `INSERT INTO run(country,rel,state,snapshot) VALUES ('vn',$1,'evaluated','{}') RETURNING id`, rel).Scan(&runID); err != nil {
		t.Fatal(err)
	}
	if err = workshop.QueryRow(ctx, `INSERT INTO artifact(stage,item_key,input_hash,recipe_hash,path,sha256) VALUES ('build',$1,$2,$2,'synthetic',$2) RETURNING id`, key, hash).Scan(&artifactID); err != nil {
		t.Fatal(err)
	}
	_, err = workshop.Exec(ctx, `INSERT INTO run_item(run_id,stage,item_key,input_hash,recipe_hash,state,batch,artifact_id)
	VALUES ($1,'build',$2,$3,$3,'done',0,$4),($1,'fetch','gap',$3,$3,'gap',0,NULL)`, runID, key, hash, artifactID)
	if err != nil {
		t.Fatal(err)
	}
	read := func(want bool) {
		t.Helper()
		data, readErr := readReleaseRow(ctx, "vn", rel, builderURL, workshopURL)
		if readErr != nil {
			t.Fatal(readErr)
		}
		var row struct {
			Rel        string            `json:"rel"`
			Extensions map[string]string `json:"extensions"`
			Run        struct {
				Complete bool `json:"complete"`
			} `json:"run"`
		}
		if readErr = json.Unmarshal(data, &row); readErr != nil || row.Rel != rel || row.Run.Complete != want || len(row.Extensions) != 2 || row.Extensions["vector"] == "" || row.Extensions["ltree"] == "" {
			t.Fatalf("release=%s err=%v", data, readErr)
		}
	}
	read(false)
	if _, err = workshop.Exec(ctx, `INSERT INTO gap(run_id,stage,item_key,kind,detail) VALUES ($1,'fetch','gap','missing_attachment','synthetic')`, runID); err != nil {
		t.Fatal(err)
	}
	read(false)
	if _, err = workshop.Exec(ctx, `UPDATE gap SET accepted_at=now() WHERE run_id=$1`, runID); err != nil {
		t.Fatal(err)
	}
	read(true)
	for _, state := range []string{"pending", "failed"} {
		if _, err = workshop.Exec(ctx, `UPDATE run_item SET state=$2 WHERE run_id=$1 AND stage='fetch'`, runID, state); err != nil {
			t.Fatal(err)
		}
		read(false)
	}
	if _, err = corpus.Exec(ctx, `UPDATE release SET build_input_hash=$1`, strings.Repeat("b", 64)); err != nil {
		t.Fatal(err)
	}
	if _, err = readReleaseRow(ctx, "vn", rel, builderURL, workshopURL); err == nil {
		t.Fatal("release for another build accepted")
	}
	var count int
	if err = corpus.QueryRow(ctx, `SELECT count(*) FROM release`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("release changed count=%d err=%v", count, err)
	}
}
