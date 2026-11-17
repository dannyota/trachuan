//go:build integration

package releasecheck

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	schema "danny.vn/trachuan/internal/corpusdb/frameworks"
	"danny.vn/trachuan/internal/embedding"
	"danny.vn/trachuan/internal/retrieval"
	"danny.vn/trachuan/internal/retrieval/lexical"
	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func frameworkSmokeFixture(t *testing.T) (*pgxpool.Pool, *pgxpool.Pool, string) {
	t.Helper()
	ctx := t.Context()
	reader := testpg.NewBuilder(t)
	raw := testpg.NewDatabase(t, "vector")
	admin, err := pgxpool.New(ctx, raw)
	if err != nil {
		t.Fatal("open invented release")
	}
	t.Cleanup(admin.Close)
	if err = schema.Apply(ctx, admin, 1024); err != nil {
		t.Fatal(err)
	}
	marshal := func(value any) []byte {
		t.Helper()
		data, marshalErr := json.Marshal(value)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		return data
	}
	query, document := embedding.QueryRecipe1(), embedding.DocumentRecipe1()
	dense := retrieval.DenseRecipe{Version: 1, Model: query.Model, Dimensions: query.Dimensions, Query: query, QueryHash: query.Hash(), Document: document, DocumentHash: document.Hash()}
	recipe := map[string]any{"version": 2, "status": "placeholder", "dense_depth": 10, "lexical_depth": 10, "rrf": retrieval.Recipe3().RRF,
		"abstention": map[string]any{"statistic": "first-hit-best-permitted-cosine", "threshold": 0.1, "status": "bootstrap-from-law"},
		"source":     map[string]any{"release": "invented", "recipe_version": 3, "calibration_sha256": strings.Repeat("a", 64)}}
	_, err = admin.Exec(ctx, `INSERT INTO release(singleton,kind,rel,schema_version,dense_recipe,chunk_recipe,lexical_recipe,retrieval_recipe,
 snapshot_recipe,parse_recipe,relate_recipe,embed_recipe,snapshot_input_hash,parse_input_hash,relate_input_hash,chunk_input_hash,embed_input_hash,
 framework_count,version_count,control_count,built_at)
 VALUES(true,'frameworks','20990101',1,$1,'{"name":"chunk","version":1,"parameters":{"schema":"chunks/1","limit":512,"prefix_format_version":1}}',$2,$3,
 '{"name":"snapshot","version":2,"parameters":{}}','{"name":"parse","version":1,"parameters":{}}',
 '{"name":"relate","version":4,"parameters":{}}','{"name":"embed","version":1,"parameters":{}}',
 repeat('a',64),repeat('a',64),repeat('a',64),repeat('a',64),repeat('a',64),1,1,1,now())`, marshal(dense), marshal(lexical.Recipe1()), marshal(recipe))
	if err != nil {
		t.Fatal(err)
	}
	_, err = admin.Exec(ctx, `INSERT INTO framework VALUES(1,'fw-alpha','Invented','Invented','{}');
 INSERT INTO framework_version VALUES(1,'fw-alpha','1','en','licensed','invented',NULL,'2099-01-01',1,NULL,NULL);
 INSERT INTO control(id,version_id,ordinal,citation,citation_norm,kind,title,status) VALUES(1,1,1,'A-1','A-1','control','Invented public title','active');
 INSERT INTO control_text VALUES(1,'Invented private title','Invented control security body','{}','{}');
 INSERT INTO body_chunk VALUES(1,1,1,0,'Invented control security body',repeat('a',64),4,(ARRAY[1::real]||array_fill(0::real,ARRAY[1023]))::vector);
 INSERT INTO public_chunk VALUES(1,1,1,'Invented public title',repeat('a',64),3,(ARRAY[1::real]||array_fill(0::real,ARRAY[1023]))::vector);
 INSERT INTO body_posting(version_id,chunk_id,token,tf) VALUES(1,1,'invented',1),(1,1,'control',1),(1,1,'security',1),(1,1,'body',1);
 INSERT INTO public_posting(version_id,chunk_id,token,tf) VALUES(1,1,'invented',1),(1,1,'public',1),(1,1,'title',1);
 UPDATE release SET versions_sealed=true;`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = admin.Exec(ctx, "GRANT USAGE ON SCHEMA public TO "+pgx.Identifier{reader.Role}.Sanitize()+"; GRANT SELECT ON ALL TABLES IN SCHEMA public TO "+pgx.Identifier{reader.Role}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	cfg, err := pgxpool.ParseConfig(raw)
	if err != nil {
		t.Fatal("parse fixture URL")
	}
	for setting, value := range map[string]string{"log_min_messages": "panic", "log_min_error_statement": "panic", "log_min_duration_statement": "-1", "log_statement": "none"} {
		if _, err = admin.Exec(ctx, "ALTER DATABASE "+pgx.Identifier{cfg.ConnConfig.Database}.Sanitize()+" SET "+setting+" = '"+value+"'"); err != nil {
			t.Fatal(err)
		}
	}
	login, err := pgx.ParseConfig(reader.URL)
	if err != nil {
		t.Fatal("parse fixture reader")
	}
	cfg.ConnConfig.User, cfg.ConnConfig.Password = login.User, login.Password
	cfg.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
	guard, err := frameworksDatabaseConfig("postgres://corpus_reader@localhost/{database}", "frameworks_20990101")
	if err != nil {
		t.Fatal(err)
	}
	cfg.AfterConnect = guard.AfterConnect
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal("open fixture reader")
	}
	t.Cleanup(pool.Close)
	return admin, pool, "frameworks_20990101"
}

func TestCheckFrameworksDatabase_SearchReadAndSafeOutput(t *testing.T) {
	admin, db, name := frameworkSmokeFixture(t)
	var valid bool
	err := admin.QueryRow(t.Context(), `SELECT
 NOT EXISTS(SELECT FROM body_chunk c WHERE c.token_count <> (SELECT coalesce(sum(p.tf),0) FROM body_posting p WHERE p.chunk_id=c.id AND p.version_id=c.version_id))
 AND NOT EXISTS(SELECT FROM public_chunk c WHERE c.token_count <> (SELECT coalesce(sum(p.tf),0) FROM public_posting p WHERE p.chunk_id=c.id AND p.version_id=c.version_id))
 AND (SELECT count(*)=1 AND bool_and(chunk_count=1 AND avg_length=4) FROM body_lexical_stats)
 AND (SELECT chunk_count=1 AND avg_length=3 FROM public_lexical_stats)
 AND (SELECT count(*)=4 AND bool_and(df=1) FROM body_lexical_df)
 AND (SELECT count(*)=3 AND bool_and(df=1) FROM public_lexical_df)`).Scan(&valid)
	if err != nil || !valid {
		t.Fatal("fixture postings or lexical statistics disagree with chunk lengths")
	}
	transport := &smokeTransport{}
	client := embedding.NewClient("http://embedder", &http.Client{Transport: transport})
	report, err := checkFrameworksDatabase(t.Context(), db, name, client)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(report)
	if err != nil || string(data) != `{"status":"ok","search_hits":1,"read_items":1}` {
		t.Fatal("unsafe smoke output")
	}
	if len(transport.calls) != 2 || transport.calls[0] != "/v1/recipe" || transport.calls[1] != "/v1/embed" {
		t.Fatal("smoke did not search through the embedder")
	}
	if _, err = db.Exec(t.Context(), `DELETE FROM control_text`); err == nil {
		t.Fatal("smoke reader wrote to database")
	}
	if _, err = checkFrameworksDatabase(t.Context(), db, "frameworks_20990102", client); err == nil {
		t.Fatal("smoke accepted wrong release identity")
	}
	if len(transport.calls) != 2 {
		t.Fatal("mismatched identity opened the engine")
	}
	if _, err = admin.Exec(t.Context(), `ALTER TABLE release DROP CONSTRAINT release_schema_version_check; UPDATE release SET schema_version=2`); err != nil {
		t.Fatal(err)
	}
	if _, err = checkFrameworksDatabase(t.Context(), db, name, client); err == nil {
		t.Fatal("smoke accepted unsupported schema")
	}
}

func TestCheckFrameworksDatabase_RefusesLogsBeforeReleaseRead(t *testing.T) {
	db, err := pgxpool.New(t.Context(), testpg.NewDatabase(t))
	if err != nil {
		t.Fatal("connect invented database")
	}
	defer db.Close()
	_, err = checkFrameworksDatabase(context.Background(), db, "frameworks_20990101", nil)
	if err == nil || err.Error() != "frameworks release-check log settings: Go type *errors.errorString" {
		t.Fatal("smoke read release before checking log settings")
	}
}

func TestCheckFrameworksDatabase_ReportsSafeEngineFailure(t *testing.T) {
	admin, db, name := frameworkSmokeFixture(t)
	if _, err := admin.Exec(t.Context(), `DELETE FROM body_posting`); err != nil {
		t.Fatal(err)
	}
	client := embedding.NewClient("http://embedder", &http.Client{Transport: &smokeTransport{}})
	_, err := checkFrameworksDatabase(t.Context(), db, name, client)
	if err == nil || err.Error() != "frameworks release-check open engine: Go type serve.failure" {
		t.Fatal("missing safe engine failure cause")
	}
}

func TestCheckFrameworksDatabase_ReportsReleaseReadSQLState(t *testing.T) {
	admin, db, name := frameworkSmokeFixture(t)
	if _, err := admin.Exec(t.Context(), `ALTER TABLE release RENAME TO invented_release`); err != nil {
		t.Fatal(err)
	}
	_, err := checkFrameworksDatabase(t.Context(), db, name, nil)
	if err == nil || err.Error() != "frameworks release-check release row: SQLSTATE 42P01" {
		t.Fatal("missing safe release read cause")
	}
}
