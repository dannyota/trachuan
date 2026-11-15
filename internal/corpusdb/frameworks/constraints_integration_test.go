//go:build integration

package frameworks

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func requireViolation(t *testing.T, err error, code, constraint string) {
	t.Helper()
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("want SQLSTATE %s, constraint %q; got %v", code, constraint, err)
	}
	if pgErr.Code != code || pgErr.ConstraintName != constraint {
		t.Fatalf("want SQLSTATE %s, constraint %q; got SQLSTATE %s, constraint %q", code, constraint, pgErr.Code, pgErr.ConstraintName)
	}
}

type violationCase struct{ query, code, constraint string }

func runViolations(t *testing.T, conn *pgx.Conn, cases map[string]violationCase) {
	t.Helper()
	ctx := context.Background()
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			tx, err := conn.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
					t.Error(rollbackErr)
				}
			}()
			_, err = tx.Exec(ctx, tc.query)
			requireViolation(t, err, tc.code, tc.constraint)
		})
	}
}

func TestSchema_RejectsConstraintViolations(t *testing.T) {
	conn := schemaDB(t)
	seedUnsealed(t, conn)
	//nolint:misspell // Match the registry field names.
	cases := map[string]violationCase{
		"duplicate citation norm":         {`INSERT INTO control(id,version_id,ordinal,citation,citation_norm,kind,title,status) VALUES(3,1,2,'a.1','A.1','control','Our title','active')`, "23505", "control_version_citation_norm_key"},
		"citation norm mismatch":          {`UPDATE control SET citation_norm='OTHER' WHERE id=1`, "23514", "control_citation_norm_check"},
		"duplicate edge tuple":            {`INSERT INTO control_mapping SELECT 2,from_code,from_version,from_citation,to_code,to_version,to_citation,relation,source,from_version_id,to_version_id,resolved FROM control_mapping`, "23505", "control_mapping_tuple_key"},
		"relation":                        {`UPDATE control_mapping SET relation='bad'`, "23514", "control_mapping_relation_check"},
		"edge source empty":               {`UPDATE control_mapping SET source=''`, "23514", "control_mapping_source_check"},
		"edge same version":               {`UPDATE control_mapping SET to_code='fw-alpha',to_version='2.1',to_citation='A.1',to_version_id=1`, "23514", "control_mapping_versions_check"},
		"edge resolved mismatch":          {`UPDATE control_mapping SET resolved=false`, "23514", "control_mapping_resolved_check"},
		"edge business key mismatch":      {`UPDATE control_mapping SET from_version='3.0'`, "23503", "control_mapping_from_version_fkey"},
		"edge citation mismatch":          {`UPDATE control_mapping SET to_citation='B.2'`, "23503", "control_mapping_to_control_fkey"},
		"gap class mismatch":              {`UPDATE quality_gap SET class='text'`, "23514", "quality_gap_class_check"},
		"gap unknown kind":                {`UPDATE quality_gap SET kind='unknown'`, "23514", "quality_gap_class_check"},
		"gap key duplicate":               {`INSERT INTO quality_gap SELECT * FROM quality_gap`, "23505", "quality_gap_pkey"},
		"gap text detail":                 {`UPDATE quality_gap SET detail='quoted document text'`, "23514", "quality_gap_detail_check"},
		"gap empty detail":                {`UPDATE quality_gap SET detail=''`, "23514", "quality_gap_detail_check"},
		"gap version missing":             {`UPDATE quality_gap SET version_id=99`, "23503", "quality_gap_version_fkey"},
		"body without control":            {`INSERT INTO control_text VALUES(99,'Invented','Invented','{}','{}')`, "23503", "control_text_control_fkey"},
		"body duplicate":                  {`INSERT INTO control_text SELECT * FROM control_text`, "23505", "control_text_pkey"},
		"duplicate framework":             {`INSERT INTO framework VALUES(3,'fw-alpha','Invented','Invented','{}')`, "23505", "framework_code_key"},
		"duplicate version":               {`INSERT INTO framework_version(id,framework_code,version,language,access,licence_kind,released,ordinal) VALUES(3,'fw-alpha','2.1','en','licensed','synthetic','2099-01-02',2)`, "23505", "framework_version_code_version_key"},
		"duplicate version order":         {`INSERT INTO framework_version(id,framework_code,version,language,access,licence_kind,released,ordinal) VALUES(3,'fw-alpha','3.0','en','licensed','synthetic','2099-01-01',1)`, "23505", "framework_version_order_key"},
		"version framework missing":       {`INSERT INTO framework_version(id,framework_code,version,language,access,licence_kind,released,ordinal) VALUES(3,'missing','3.0','en','licensed','synthetic','2099-01-02',2)`, "23503", "framework_version_framework_fkey"},
		"access invalid":                  {`INSERT INTO framework_version(id,framework_code,version,language,access,licence_kind,released,ordinal) VALUES(3,'fw-alpha','3.0','en','other','synthetic','2099-01-02',2)`, "23514", "framework_version_access_check"},
		"version ordinal zero":            {`INSERT INTO framework_version(id,framework_code,version,language,access,licence_kind,released,ordinal) VALUES(3,'fw-alpha','3.0','en','licensed','synthetic','2099-01-02',0)`, "23514", "framework_version_ordinal_check"},
		"version self lineage":            {`INSERT INTO framework_version(id,framework_code,version,language,access,licence_kind,released,ordinal,supersedes_id) VALUES(3,'fw-alpha','3.0','en','licensed','synthetic','2099-01-02',2,3)`, "23514", "framework_version_supersedes_check"},
		"version cross framework lineage": {`INSERT INTO framework_version(id,framework_code,version,language,access,licence_kind,released,ordinal,amends_id) VALUES(3,'fw-alpha','3.0','en','licensed','synthetic','2099-01-02',2,2)`, "23503", "framework_version_amends_fkey"},
		"control self parent":             {`UPDATE control SET parent_id=id WHERE id=1`, "23514", "control_parent_check"},
		"control cross version parent":    {`UPDATE control SET parent_id=2 WHERE id=1`, "23503", "control_parent_fkey"},
		"control lineage missing":         {`UPDATE control SET supersedes_id=99 WHERE id=1`, "23503", "control_supersedes_fkey"},
		"control self lineage":            {`UPDATE control SET amends_id=id WHERE id=1`, "23514", "control_amends_check"},
		"control order duplicate":         {`INSERT INTO control(id,version_id,ordinal,citation,citation_norm,kind,title,status) VALUES(3,1,1,'A.2','A.2','control','Our title','active')`, "23505", "control_sibling_ordinal_key"},
		"control ordinal zero":            {`UPDATE control SET ordinal=0 WHERE id=1`, "23514", "control_ordinal_check"},
		"source hash":                     {`UPDATE source_file SET sha256='bad'`, "23514", "source_file_sha256_check"},
		"source size":                     {`UPDATE source_file SET size=-1`, "23514", "source_file_size_check"},
		"source role":                     {`UPDATE source_file SET role='bad'`, "23514", "source_file_role_check"},
		"source text source":              {`UPDATE source_file SET text_source='bad'`, "23514", "source_file_text_source_check"},
		"source duplicate":                {`INSERT INTO source_file SELECT 2,version_id,name,role,sha256,size,licence_kind,text_source,text_complete FROM source_file`, "23505", "source_file_identity_key"},
		"source version missing":          {`UPDATE source_file SET version_id=99`, "23503", "source_file_version_fkey"},
		"release duplicate":               {`INSERT INTO release SELECT singleton,kind,'20990102',schema_version,dense_recipe,chunk_recipe,lexical_recipe,retrieval_recipe,framework_count,version_count,control_count,built_at,versions_sealed FROM release`, "23505", "release_pkey"},
		"release singleton false":         {`UPDATE release SET singleton=false`, "23514", "release_singleton_check"},
		"release kind":                    {`UPDATE release SET kind='law'`, "23514", "release_kind_check"},
		"release schema version":          {`UPDATE release SET schema_version=7`, "23514", "release_schema_version_check"},
		"release framework count":         {`UPDATE release SET framework_count=-1`, "23514", "release_framework_count_check"},
		"release version count":           {`UPDATE release SET version_count=-1`, "23514", "release_version_count_check"},
		"release count":                   {`UPDATE release SET control_count=-1`, "23514", "release_control_count_check"},
	}
	for _, table := range []string{"body_chunk", "public_chunk"} {
		cases[table+" control missing"] = violationCase{"UPDATE " + table + " SET control_id=99", "23503", table + "_control_fkey"}
		cases[table+" version mismatch"] = violationCase{"UPDATE " + table + " SET control_id=2", "23503", table + "_control_fkey"}
		cases[table+" empty content"] = violationCase{"UPDATE " + table + " SET content=''", "23514", table + "_content_check"}
		cases[table+" input hash"] = violationCase{"UPDATE " + table + " SET input_sha256='bad'", "23514", table + "_input_sha256_check"}
		cases[table+" token count"] = violationCase{"UPDATE " + table + " SET token_count=0", "23514", table + "_token_count_check"}
	}
	for _, table := range []string{"body_posting", "public_posting"} {
		cases[table+" version mismatch"] = violationCase{"UPDATE " + table + " SET version_id=2", "23503", table + "_chunk_fkey"}
		cases[table+" chunk missing"] = violationCase{"UPDATE " + table + " SET chunk_id=99", "23503", table + "_chunk_fkey"}
		cases[table+" empty token"] = violationCase{"UPDATE " + table + " SET token=''", "23514", table + "_token_check"}
		cases[table+" frequency"] = violationCase{"UPDATE " + table + " SET tf=0", "23514", table + "_tf_check"}
		cases[table+" duplicate"] = violationCase{"INSERT INTO " + table + " SELECT * FROM " + table, "23505", table + "_pkey"}
	}
	cases["body chunk ordinal"] = violationCase{`UPDATE body_chunk SET ordinal=-1`, "23514", "body_chunk_ordinal_check"}
	cases["body chunk control ordinal duplicate"] = violationCase{`INSERT INTO body_chunk SELECT 2,version_id,control_id,ordinal,content,input_sha256,token_count,embedding FROM body_chunk`, "23505", "body_chunk_control_ordinal_key"}
	cases["public chunk control duplicate"] = violationCase{`INSERT INTO public_chunk SELECT 2,version_id,control_id,content,input_sha256,token_count,embedding FROM public_chunk`, "23505", "public_chunk_control_key"}
	cases["body chunk primary key"] = violationCase{`INSERT INTO body_chunk SELECT id,2,2,ordinal,content,input_sha256,token_count,embedding FROM body_chunk`, "23505", "body_chunk_pkey"}
	cases["public chunk primary key"] = violationCase{`INSERT INTO public_chunk SELECT id,2,2,content,input_sha256,token_count,embedding FROM public_chunk`, "23505", "public_chunk_pkey"}
	for _, table := range []string{"framework", "source_file", "control", "body_chunk", "public_chunk", "control_mapping"} {
		cases[table+" positive id"] = violationCase{"UPDATE " + table + " SET id=0 WHERE id=1", "23514", table + "_id_check"}
	}
	//nolint:misspell // Match the registry field names.
	cases["framework_version positive id"] = violationCase{`INSERT INTO framework_version(id,framework_code,version,language,access,licence_kind,released,ordinal) VALUES(0,'fw-alpha','3.0','en','licensed','synthetic','2099-01-02',2)`, "23514", "framework_version_id_check"}
	for _, end := range []string{"from", "to"} {
		cases[end+" mapping code syntax"] = violationCase{"UPDATE control_mapping SET " + end + "_code='FW-alpha'", "23514", "control_mapping_" + end + "_code_check"}
		cases[end+" mapping version syntax"] = violationCase{"UPDATE control_mapping SET " + end + "_version='1..0'", "23514", "control_mapping_" + end + "_version_check"}
		cases[end+" mapping citation alphabet"] = violationCase{"UPDATE control_mapping SET " + end + "_citation=' A.1'", "23514", "control_mapping_" + end + "_citation_check"}
	}
	for _, recipe := range []string{"dense_recipe", "chunk_recipe", "lexical_recipe", "retrieval_recipe"} {
		cases[recipe+" shape"] = violationCase{"UPDATE release SET " + recipe + "='[]'", "23514", "release_" + recipe + "_check"}
	}
	runViolations(t, conn, cases)
}

func TestSchema_RequiresExplicitAccess(t *testing.T) {
	conn := schemaDB(t)
	seedUnsealed(t, conn)
	//nolint:misspell // Match the registry field names.
	_, err := conn.Exec(context.Background(), `INSERT INTO framework_version(id,framework_code,version,language,licence_kind,released,ordinal) VALUES(3,'fw-alpha','3.0','en','synthetic','2099-01-02',2)`)
	requireViolation(t, err, "23502", "")
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatal(err)
	}
	if pgErr.TableName != "framework_version" || pgErr.ColumnName != "access" {
		t.Fatalf("want framework_version.access NOT NULL, got %s.%s", pgErr.TableName, pgErr.ColumnName)
	}
}
