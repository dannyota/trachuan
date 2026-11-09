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

func TestApply_CreatesSchemaVersion1(t *testing.T) {
	conn := schemaDB(t)
	ctx := context.Background()
	for _, table := range []string{"document", "source_file", "provision", "provision_validity", "chunk", "lexical_df", "relation", "coverage_gap", "taxonomy_node", "taxonomy_assignment", "release"} {
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
	_, err := conn.Exec(ctx, `INSERT INTO document (id,country,key,number,number_norm,title,doc_type,issuer,raw_issue_date,raw_eff_from,raw_eff_to,status_code,status_name,status_class,is_current,is_consolidated,content_state,fetch_sha256) VALUES (1,'vn','vn:test','n','n','t','t','i','','','','','','unknown',false,false,'indexed',$1)`, hash)
	if err != nil {
		t.Fatal(err)
	}
	_, err = conn.Exec(ctx, `INSERT INTO release VALUES (true,'vn','r',1,'','{}','{}','{}','{}','{}',$1,$1,$1,'{}',now())`, hash)
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
			_, err := conn.Exec(ctx, query)
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) || !strings.HasPrefix(pgErr.Code, "23") {
				t.Fatalf("expected SQLSTATE 23, got %v", err)
			}
		})
	}
}
