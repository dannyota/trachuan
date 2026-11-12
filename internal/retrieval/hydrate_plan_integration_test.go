//go:build integration

package retrieval

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type hydrationPlanTrace struct {
	t                *testing.T
	count, relations int
	explaining       bool
}

func (trace *hydrationPlanTrace) TraceQueryStart(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if trace.explaining || (!strings.HasPrefix(data.SQL, "WITH RECURSIVE") && !strings.HasPrefix(data.SQL, "SELECT c.id,c.ordinal") && !strings.Contains(data.SQL, "relation ")) {
		return ctx
	}
	trace.explaining = true
	defer func() { trace.explaining = false }()
	var raw []byte
	if err := conn.QueryRow(ctx, "EXPLAIN (FORMAT JSON) "+data.SQL, data.Args...).Scan(&raw); err != nil {
		trace.t.Errorf("explain hydration: %v", err)
		return ctx
	}
	var plan any
	if err := json.Unmarshal(raw, &plan); err != nil {
		trace.t.Errorf("decode hydration plan: %v", err)
		return ctx
	}
	trace.count++
	if checkHydrationPlan(trace.t, plan) {
		trace.relations++
	}
	return ctx
}

func (*hydrationPlanTrace) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func checkHydrationPlan(t *testing.T, value any) bool {
	t.Helper()
	found := false
	switch v := value.(type) {
	case []any:
		for _, child := range v {
			if checkHydrationPlan(t, child) {
				found = true
			}
		}
	case map[string]any:
		table, _ := v["Relation Name"].(string)
		node, _ := v["Node Type"].(string)
		if table == "provision" || table == "chunk" || table == "relation" {
			if node == "Seq Scan" || (strings.Contains(node, "Index") && v["Index Cond"] == nil) {
				t.Errorf("unbounded %s on %s", node, table)
			}
			if table == "relation" {
				found = true
			}
		}
		for _, child := range v {
			if checkHydrationPlan(t, child) {
				found = true
			}
		}
	}
	return found
}

func TestHydrate_StatementsAvoidFullScans(t *testing.T) {
	db, e, _ := fixture(t)
	ctx := context.Background()
	statements := []string{
		`INSERT INTO document (id,country,key,number,number_norm,title,doc_type,issuer,class,source,validity_conflict,raw_issue_date,raw_eff_from,raw_eff_to,status_code,status_name,status_class,status_basis,is_current,is_consolidated,content_state,fetch_sha256,text_source,text_complete) SELECT n,'vn','vn:filler-'||n,'n','n','title','law','issuer','relation_context','vbpl',false,'','','','','','in_force','source_status',true,false,'indexed',repeat('a',64),'source_tree',true FROM generate_series(100,129) n`,
		`INSERT INTO provision (id,document_id,node_id,kind,label,heading,ordinal,preorder,path,text,anchor) SELECT 10000+n,100+(n-1)/100,'p'||n,'dieu','p','',n,n,('p'||n)::ltree,'p','p'||n FROM generate_series(1,3000) n`,
		`INSERT INTO chunk (id,document_id,provision_id,ordinal,context_prefix,content,spans,input_sha256,token_count,embedding,sparse) SELECT 10000+n,100+(n-1)/100,10000+n,(n-1)%100,'','p','[]',repeat('a',64),1,c.embedding,c.sparse FROM generate_series(1,3000) n CROSS JOIN chunk c WHERE c.id=1`,
		`INSERT INTO relation (id,source_document_id,target_document_id,source_key,target_key,source_number,target_number,kind,raw_code,raw_label,evidence,method,artifact_sha256,ordinal) SELECT n,100+(n%30),100+((n+1)%30),'vn:filler-'||(100+(n%30)),'vn:filler-'||(100+((n+1)%30)),'n','n','cites','','','','',repeat('a',64),n FROM generate_series(1,10000) n`,
		`INSERT INTO relation (id,source_document_id,target_document_id,source_key,target_key,source_number,target_number,kind,raw_code,raw_label,evidence,method,artifact_sha256,ordinal) VALUES (10001,1,100,'vn:1','vn:filler-100','n','n','cites','','','','',repeat('a',64),0)`,
		`ANALYZE document`, `ANALYZE provision`, `ANALYZE chunk`, `ANALYZE relation`,
	}
	for _, sql := range statements {
		if _, err := db.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	trace := &hydrationPlanTrace{t: t}
	cfg := db.Config()
	cfg.ConnConfig.RuntimeParams["enable_seqscan"] = "on"
	cfg.ConnConfig.Tracer = trace
	tracedDB, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer tracedDB.Close()
	e.db = tracedDB
	for _, mode := range []Mode{"exact", "ann"} {
		t.Run(string(mode), func(t *testing.T) {
			trace.count, trace.relations = 0, 0
			a := collectFixture(t, e, Request{Query: "ngân hàng", Mode: mode}, Depths{200, 4})
			if mode == "ann" && !a.Primary.Fallback {
				t.Fatal("ANN did not fall back to exact search")
			}
			if len(a.Primary.Dense) == 0 || a.Primary.Dense[0].Identity.DocumentKey != "vn:1" {
				t.Fatalf("candidate was not hydrated: %+v", a.Primary.Dense)
			}
			docs, err := e.Documents(ctx, []string{"vn:1"})
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := docs["vn:1"]; !ok {
				t.Fatal("document metadata was not loaded")
			}
			if trace.count < 5 || trace.relations == 0 {
				t.Fatalf("explained %d hydration statements, including %d relation statements", trace.count, trace.relations)
			}
		})
	}
}
