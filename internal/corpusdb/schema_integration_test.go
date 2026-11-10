//go:build integration

package corpusdb

import (
	"context"
	"errors"
	"strings"
	"testing"

	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func schemaDB(t *testing.T) *pgx.Conn {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, testpg.NewDatabase(t, "vector", "ltree"))
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

func TestApply_CreatesSchemaVersion3(t *testing.T) {
	conn := schemaDB(t)
	ctx := context.Background()
	for _, table := range []string{"document", "source_file", "provision", "provision_validity", "chunk", "lexical_df", "relation", "coverage_gap", "taxonomy_node", "taxonomy_assignment", "validity_decision", "release"} {
		var exists bool
		if err := conn.QueryRow(ctx, `SELECT to_regclass($1) IS NOT NULL`, "public."+table).Scan(&exists); err != nil || !exists {
			t.Fatalf("table %s: exists=%v error=%v", table, exists, err)
		}
	}
	for name, want := range map[string]string{"embedding": "vector(1024)", "sparse": "sparsevec(1048576)"} {
		var typ string
		if err := conn.QueryRow(ctx, `SELECT format_type(atttypid, atttypmod) FROM pg_attribute WHERE attrelid='chunk'::regclass AND attname=$1`, name).Scan(&typ); err != nil || typ != want {
			t.Fatalf("%s type = %q, error = %v", name, typ, err)
		}
	}
	var definition string
	if err := conn.QueryRow(ctx, `SELECT indexdef FROM pg_indexes WHERE tablename='chunk' AND indexdef LIKE '%USING hnsw%'`).Scan(&definition); err != nil {
		t.Fatal(err)
	}
	for _, part := range []string{"embedding vector_cosine_ops", "m='16'", "ef_construction='64'"} {
		if !strings.Contains(definition, part) {
			t.Errorf("index %q lacks %q", definition, part)
		}
	}
}

func TestApply_ConstraintsRejectBadRows(t *testing.T) {
	conn := schemaDB(t)
	ctx := context.Background()
	hash := strings.Repeat("a", 64)
	_, err := conn.Exec(ctx, `INSERT INTO document (id,country,key,number,number_norm,title,doc_type,issuer,class,source,validity_conflict,raw_issue_date,raw_eff_from,raw_eff_to,status_code,status_name,status_class,status_basis,is_current,is_consolidated,content_state,fetch_sha256) VALUES (1,'vn','vn:test','n','n','t','t','i','primary','vbpl',false,'','','','','','unknown','source_status',false,false,'indexed',$1)`, hash)
	if err != nil {
		t.Fatal(err)
	}
	_, err = conn.Exec(ctx, `INSERT INTO release VALUES (true,'vn','r',3,'','{}','{}','{}','{}','{}',$1,$1,$1,'{}',now())`, hash)
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		"second release": `INSERT INTO release SELECT * FROM release`,
		"country":        `UPDATE document SET country='us' WHERE id=1`,
		"hash":           `UPDATE document SET fetch_sha256='bad' WHERE id=1`,
		"dates":          `UPDATE document SET eff_from='2025-01-02',eff_to='2025-01-01' WHERE id=1`,
		"relation kind":  `INSERT INTO relation (id,source_key,source_number,target_number,kind,raw_code,raw_label,evidence,method,artifact_sha256,ordinal) VALUES (1,'vn:test','','target','bad','','','','','` + hash + `',0)`,
		"self parent":    `INSERT INTO provision (id,document_id,parent_id,node_id,kind,label,heading,ordinal,preorder,path,text,anchor) VALUES (1,1,1,'n','other','','',1,1,'n1','','a')`,
	}
	for name, query := range cases {
		t.Run(name, func(t *testing.T) {
			_, execErr := conn.Exec(ctx, query)
			var pgErr *pgconn.PgError
			if !errors.As(execErr, &pgErr) || !strings.HasPrefix(pgErr.Code, "23") {
				t.Fatalf("expected SQLSTATE 23, got %v", execErr)
			}
		})
	}
}

func TestSchema_Version3Constraints(t *testing.T) {
	conn := schemaDB(t)
	ctx := context.Background()
	hash := strings.Repeat("a", 64)
	_, err := conn.Exec(ctx, `INSERT INTO document(id,country,key,number,number_norm,title,doc_type,issuer,class,source,raw_issue_date,raw_eff_from,raw_eff_to,status_code,status_name,status_class,status_basis,validity_conflict,is_current,is_consolidated,content_state,fetch_sha256) VALUES(1,'vn','vn:test','','','','','','primary','vbpl','','','','','','unknown','source_status',false,false,false,'indexed',$1)`, hash)
	if err != nil {
		t.Fatal(err)
	}
	_, err = conn.Exec(ctx, `INSERT INTO validity_decision(document_id,ordinal,layer,rule_version,before,after) VALUES(1,1,'source','1',$1,$1)`, `{"status_class":"unknown","is_current":false,"status_basis":"source_status","eff_from":null,"eff_to":null,"validity_conflict":false}`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = conn.Exec(ctx, `INSERT INTO relation(id,source_key,source_number,target_number,kind,raw_code,raw_label,evidence,method,artifact_sha256,ordinal) VALUES(1,'vn:test','','','unknown','','','','',$1,0)`, hash)
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		"class": `UPDATE document SET class='bad'`, "class null": `UPDATE document SET class=NULL`,
		"source": `UPDATE document SET source='bad'`, "source null": `UPDATE document SET source=NULL`,
		"basis": `UPDATE document SET status_basis='bad'`, "conflict null": `UPDATE document SET validity_conflict=NULL`,
		"document foreign key": `INSERT INTO validity_decision SELECT 2,2,layer,rule_version,before,after,actor_key,effect_date,relation_id,anchor,quote,artifact_sha256 FROM validity_decision`,
		"duplicate key":        `INSERT INTO validity_decision SELECT * FROM validity_decision`,
		"ordinal zero":         `UPDATE validity_decision SET ordinal=0`, "ordinal nine": `UPDATE validity_decision SET ordinal=9`,
		"layer": `UPDATE validity_decision SET layer='bad'`, "quote": `UPDATE validity_decision SET quote=repeat('x',401)`,
		"hash": `UPDATE validity_decision SET artifact_sha256='bad'`, "B actor": `UPDATE validity_decision SET layer='B',effect_date='2025-01-01'`,
		"B date":               `UPDATE validity_decision SET layer='B',actor_key='vn:vbpl:1'`,
		"C actor":              `UPDATE validity_decision SET layer='C',effect_date='2025-01-01',relation_id=1`,
		"C date":               `UPDATE validity_decision SET layer='C',actor_key='vn:vbpl:1',relation_id=1`,
		"C relation":           `UPDATE validity_decision SET layer='C',actor_key='vn:vbpl:1',effect_date='2025-01-01'`,
		"relation foreign key": `UPDATE validity_decision SET relation_id=99`,
		"release version":      `INSERT INTO release VALUES(true,'vn','r',2,'','{}','{}','{}','{}','{}','` + hash + `','` + hash + `','` + hash + `','{}',now())`,
	}
	for _, column := range []string{"before", "after"} {
		for name, value := range map[string]string{
			"empty": `{}`, "missing": `{"status_class":"unknown","is_current":false,"status_basis":"source_status","eff_from":null,"eff_to":null}`,
			"extra": `{"status_class":"unknown","is_current":false,"status_basis":"source_status","eff_from":null,"eff_to":null,"validity_conflict":false,"extra":1}`, "non_object": `[]`,
		} {
			cases[column+" "+name] = "UPDATE validity_decision SET " + column + "='" + value + "'::jsonb"
		}
	}

	for name, query := range cases {
		t.Run(name, func(t *testing.T) {
			_, execErr := conn.Exec(ctx, query)
			var pgErr *pgconn.PgError
			if !errors.As(execErr, &pgErr) || !strings.HasPrefix(pgErr.Code, "23") {
				t.Fatalf("expected constraint error, got %v", execErr)
			}
		})
	}
	for _, query := range []string{`UPDATE document SET class='relation_context',source='chinhphu',status_basis='derived_from_text',validity_conflict=true`, `UPDATE document SET class='secondary',status_basis='manual_correction'`, `UPDATE validity_decision SET ordinal=8,quote=repeat('x',400)`} {
		if _, execErr := conn.Exec(ctx, query); execErr != nil {
			t.Fatal(execErr)
		}
	}
	_, err = conn.Exec(ctx, `INSERT INTO release VALUES(true,'vn','r',3,'','{}','{}','{}','{}','{}',$1,$1,$1,'{}',now())`, hash)
	if err != nil {
		t.Fatal(err)
	}
}
