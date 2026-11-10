//go:build integration

package membership

import (
	"context"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/testdb"
	"danny.vn/trachuan/apps/corpus/migrations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func TestScopeMigration_Backfill(t *testing.T) {
	ctx := context.Background()
	url := testdb.NewEmpty(t)
	config, err := pgx.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	db := stdlib.OpenDB(*config)
	defer func() { _ = db.Close() }()
	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = provider.UpTo(ctx, 3); err != nil {
		t.Fatal(err)
	}
	var artifact int64
	if err = db.QueryRowContext(ctx, `INSERT INTO artifact(stage,item_key,input_hash,recipe_hash,path,sha256) VALUES ('fetch','1','a','b','p','s') RETURNING id`).Scan(&artifact); err != nil {
		t.Fatal(err)
	}
	_, err = db.ExecContext(ctx, `INSERT INTO document(artifact_id,key,vbpl_id,number,title,doc_type,issuer,issue_date,eff_from,eff_to,status_code,status_name,is_consolidated,has_content) VALUES ($1,'vn:vbpl:1','1','','','','','','','','','',false,false)`, artifact)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = provider.Up(ctx); err != nil {
		t.Fatal(err)
	}
	var country, source, id, vbpl string
	if err := db.QueryRowContext(ctx, `SELECT country,source,source_id,vbpl_id FROM document WHERE artifact_id=$1`, artifact).Scan(&country, &source, &id, &vbpl); err != nil {
		t.Fatal(err)
	}
	if country != "vn" || source != "vbpl" || id != "1" || vbpl != "1" {
		t.Fatalf("backfill: %q %q %q %q", country, source, id, vbpl)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO fetched_file(artifact_id,name,sha256,size,path) VALUES ($1,'a','b',1,'p')`, artifact); err != nil {
		t.Fatal(err)
	}
	var fileSource, role string
	if err := db.QueryRowContext(ctx, `SELECT source,role FROM fetched_file WHERE artifact_id=$1`, artifact).Scan(&fileSource, &role); err != nil {
		t.Fatal(err)
	}
	if fileSource != "vbpl" || role != "unknown" {
		t.Fatalf("file provenance = %q %q", fileSource, role)
	}
	for _, query := range []string{
		`UPDATE fetched_file SET role='bad' WHERE artifact_id=$1`,
		`UPDATE fetched_file SET source='other' WHERE artifact_id=$1`,
	} {
		if _, err := db.ExecContext(ctx, query, artifact); err == nil {
			t.Errorf("accepted %s", query)
		}
	}
	for _, query := range []string{
		`UPDATE document SET key='vn:vbpl:2' WHERE artifact_id=$1`,
		`UPDATE document SET vbpl_id=NULL WHERE artifact_id=$1`,
		`UPDATE document SET vbpl_id='2' WHERE artifact_id=$1`,
		`UPDATE document SET source='other',key='vn:other:1' WHERE artifact_id=$1`,
	} {
		if _, err := db.ExecContext(ctx, query, artifact); err == nil {
			t.Errorf("accepted %s", query)
		}
	}
}
