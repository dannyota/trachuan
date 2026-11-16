//go:build integration

package frameworks

import (
	"context"
	"strings"
	"testing"

	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5"
)

func schemaDB(t *testing.T) *pgx.Conn {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, testpg.NewDatabase(t, "vector"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := conn.Close(ctx); err != nil {
			t.Error(err)
		}
	})
	if err := Apply(ctx, conn, 1024); err != nil {
		t.Fatal(err)
	}
	return conn
}

func seedUnsealed(t *testing.T, conn *pgx.Conn) {
	t.Helper()
	//nolint:misspell // Match the registry field names.
	_, err := conn.Exec(context.Background(), releaseFixtureSQL+`
 INSERT INTO framework(id,code,name,publisher,aliases) VALUES
 (1,'fw-alpha','Framework Alpha','Invented publisher','{alpha}'),(2,'fw-beta','Framework Beta','Invented publisher','{beta}');
 INSERT INTO framework_version(id,framework_code,version,language,access,licence_kind,released,ordinal) VALUES
 (1,'fw-alpha','2.1','en','licensed','synthetic','2099-01-01',1),
 (2,'fw-beta','1.0','en','free','public-domain','2099-01-01',1);
 INSERT INTO control(id,version_id,ordinal,citation,citation_norm,kind,title,status) VALUES
 (1,1,1,'A.1','A.1','control','Our alpha title','active'),
 (2,2,1,'B.1','B.1','control','Our beta title','active');
 INSERT INTO control_text(control_id,title_original,body,notes,tables) VALUES(1,'Invented title','Invented body','{}','{}');
 INSERT INTO source_file(id,version_id,name,role,sha256,size,licence_kind,text_source,text_complete)
 VALUES(1,1,'alpha.txt','text',repeat('a',64),13,'synthetic','structured',true);
 INSERT INTO body_chunk(id,version_id,control_id,ordinal,content,input_sha256,token_count,embedding)
 VALUES(1,1,1,0,'Invented body',repeat('a',64),2,array_fill(0::real,ARRAY[1024])::vector);
 INSERT INTO public_chunk(id,version_id,control_id,content,input_sha256,token_count,embedding)
 VALUES(1,1,1,'A.1 Our alpha title control',repeat('b',64),5,array_fill(0::real,ARRAY[1024])::vector);
 INSERT INTO body_posting(version_id,chunk_id,token,tf) VALUES(1,1,'invented',1);
 INSERT INTO public_posting(version_id,chunk_id,token,tf) VALUES(1,1,'alpha',1);
 INSERT INTO control_mapping(id,from_code,from_version,from_citation,to_code,to_version,to_citation,relation,source,from_version_id,to_version_id,resolved)
 VALUES(1,'fw-alpha','2.1','A.1','fw-beta','1.0','B.1','related','invented',1,2,true);
 INSERT INTO quality_gap(kind,class,version_id,subject,detail,blocks) VALUES('title_missing','public',1,'A.1','1:0',true);`)
	if err != nil {
		t.Fatal(err)
	}
}

func seed(t *testing.T, conn *pgx.Conn) {
	t.Helper()
	seedUnsealed(t, conn)
	if _, err := conn.Exec(context.Background(), `UPDATE release SET versions_sealed=true;
 INSERT INTO mapping_evidence(mapping_id,evidence,licensed_version_ids) VALUES(1,'Invented evidence','{1}');`); err != nil {
		t.Fatal(err)
	}
}

func TestApply_IdempotentAndSeparate(t *testing.T) {
	conn := schemaDB(t)
	seed(t, conn)
	if err := Apply(context.Background(), conn, 1024); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := conn.QueryRow(context.Background(), `SELECT count(*) FROM control`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("controls = %d, error = %v", count, err)
	}
	for _, table := range []string{"framework", "framework_version", "source_file", "control", "control_text", "body_chunk", "public_chunk", "body_posting", "public_posting", "control_mapping", "mapping_evidence", "quality_gap", "release"} {
		var exists bool
		if err := conn.QueryRow(context.Background(), `SELECT to_regclass($1) IS NOT NULL`, "public."+table).Scan(&exists); err != nil || !exists {
			t.Fatalf("table %s: %v %v", table, exists, err)
		}
	}
	var law, ann bool
	if err := conn.QueryRow(context.Background(), `SELECT to_regclass('document') IS NOT NULL, EXISTS(SELECT FROM pg_indexes WHERE schemaname='public' AND (indexdef LIKE '%USING hnsw%' OR indexdef LIKE '%USING ivfflat%'))`).Scan(&law, &ann); err != nil || law || ann {
		t.Fatalf("law=%v ANN=%v error=%v", law, ann, err)
	}
	for _, table := range []string{"body_chunk", "public_chunk"} {
		var typ string
		if err := conn.QueryRow(context.Background(), `SELECT format_type(atttypid,atttypmod) FROM pg_attribute WHERE attrelid=$1::regclass AND attname='embedding'`, table).Scan(&typ); err != nil || typ != "vector(1024)" {
			t.Fatalf("%s vector = %q, error = %v", table, typ, err)
		}
	}
}

func TestSchema_IdentityAlphabet(t *testing.T) {
	conn := schemaDB(t)
	seedUnsealed(t, conn)
	cases := []struct {
		column, constraint string
		bad, good          []string
	}{
		{"citation", "control_citation_check", []string{"", " A.1", "A.1 ", "A  1", "A\t1", "A\n1", "Đ.1", strings.Repeat("a", 65), "A\x7f"}, []string{"A.2", "AC-2(1)", "A 1", "!~", strings.Repeat("X", 64)}},
		{"code", "framework_code_check", []string{"", "FW-alpha", "1alpha", "fw--alpha", "fw-alpha-", "fw_alpha", "fw:alpha", "fw-alpha\n"}, []string{"fw-alpha3", "a", "fw-alpha2"}},
		{"version", "framework_version_version_check", []string{"", "V1", ".1", "1.", "1..0", "1_0", "1:0", "1.0\n"}, []string{"2.2", "1", "1.0+alpha-2"}},
	}
	for _, tc := range cases {
		for _, group := range []struct {
			values []string
			valid  bool
		}{{tc.bad, false}, {tc.good, true}} {
			for _, value := range group.values {
				t.Run(tc.column+"/"+value, func(t *testing.T) {
					tx, err := conn.Begin(context.Background())
					if err != nil {
						t.Fatal(err)
					}
					defer func() {
						if rollbackErr := tx.Rollback(context.Background()); rollbackErr != nil {
							t.Error(rollbackErr)
						}
					}()
					var query string
					switch tc.column {
					case "citation":
						query = `INSERT INTO control(id,version_id,ordinal,citation,citation_norm,kind,title,status) VALUES(3,1,2,$1,upper(replace($1,' ','')),'control','Our title','active')`
					case "code":
						query = `INSERT INTO framework(id,code,name,publisher,aliases) VALUES(3,$1,'Invented','Invented','{}')`
					case "version":
						//nolint:misspell // Match the registry field names.
						query = `INSERT INTO framework_version(id,framework_code,version,language,access,licence_kind,released,ordinal) VALUES(3,'fw-alpha',$1,'en','licensed','synthetic','2099-01-02',2)`
					}
					_, err = tx.Exec(context.Background(), query, value)
					if group.valid {
						if err != nil {
							t.Fatal(err)
						}
						return
					}
					requireViolation(t, err, "23514", tc.constraint)
				})
			}
		}
	}
}
