//go:build integration

package retrieval

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

type explainHydrationTx struct {
	pgx.Tx
	t     *testing.T
	count int
}

func (tx *explainHydrationTx) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	tx.t.Helper()
	var raw []byte
	if err := tx.Tx.QueryRow(ctx, "EXPLAIN (FORMAT JSON) "+sql, args...).Scan(&raw); err != nil {
		tx.t.Fatalf("explain hydration: %v", err)
	}
	var plan any
	if err := json.Unmarshal(raw, &plan); err != nil {
		tx.t.Fatal(err)
	}
	checkHydrationPlan(tx.t, plan)
	tx.count++
	return tx.Tx.Query(ctx, sql, args...)
}

func checkHydrationPlan(t *testing.T, value any) {
	t.Helper()
	switch v := value.(type) {
	case []any:
		for _, child := range v {
			checkHydrationPlan(t, child)
		}
	case map[string]any:
		table, _ := v["Relation Name"].(string)
		node, _ := v["Node Type"].(string)
		if table == "provision" || table == "chunk" {
			if node == "Seq Scan" || (strings.Contains(node, "Index") && v["Index Cond"] == nil) {
				t.Errorf("unbounded %s on %s", node, table)
			}
		}
		for _, child := range v {
			checkHydrationPlan(t, child)
		}
	}
}

func TestHydrate_StatementsAvoidFullScans(t *testing.T) {
	db, _, _ := fixture(t)
	ctx := context.Background()
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	statements := []string{
		`SET LOCAL enable_seqscan = on`,
		`UPDATE document SET class='relation_context' WHERE id=1`,
		`INSERT INTO document (id,country,key,number,number_norm,title,doc_type,issuer,class,source,validity_conflict,raw_issue_date,raw_eff_from,raw_eff_to,status_code,status_name,status_class,status_basis,is_current,is_consolidated,content_state,fetch_sha256) SELECT n,'vn','vn:filler-'||n,'n','n','title','law','issuer','primary','vbpl',false,'','','','','','in_force','source_status',true,false,'indexed',repeat('a',64) FROM generate_series(100,129) n`,
		`INSERT INTO provision (id,document_id,node_id,kind,label,heading,ordinal,preorder,path,text,anchor) SELECT 10000+n,100+(n-1)/100,'p'||n,'dieu','p','',n,n,('p'||n)::ltree,'p','p'||n FROM generate_series(1,3000) n`,
		`INSERT INTO chunk (id,document_id,provision_id,ordinal,context_prefix,content,spans,input_sha256,token_count,embedding,sparse) SELECT 10000+n,100+(n-1)/100,10000+n,(n-1)%100,'','p','[]',repeat('a',64),1,c.embedding,c.sparse FROM generate_series(1,3000) n CROSS JOIN chunk c WHERE c.id=1`,
		`ANALYZE document`, `ANALYZE provision`, `ANALYZE chunk`, `ANALYZE relation`,
	}
	for _, sql := range statements {
		if _, err = tx.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	traced := &explainHydrationTx{Tx: tx, t: t}
	values := make([]float32, 1024)
	values[0] = 1
	g := &ArmGroup{Dense: []Candidate{{ChunkID: 1}}}
	if err := hydrate(ctx, traced, g, denseVector(values)); err != nil {
		t.Fatal(err)
	}
	if len(g.Dense) != 1 || g.Dense[0].Identity.DocumentKey != "vn:1" {
		t.Fatalf("context candidate was not hydrated: %+v", g.Dense)
	}
	if traced.count < 4 {
		t.Fatalf("explained only %d hydration statements", traced.count)
	}
}
