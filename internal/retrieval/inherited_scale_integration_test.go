//go:build integration

package retrieval

import (
	"context"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestHydrate_InheritedEvidenceIsBounded(t *testing.T) {
	db, engine, _ := fixture(t)
	ctx := context.Background()
	_, err := db.Exec(ctx, `INSERT INTO provision (id,document_id,parent_id,node_id,kind,label,heading,ordinal,preorder,path,text,anchor) SELECT 1000+n,1,11,'clause-'||n,'khoan','clause','',n+10,n+10,('d1.c'||n)::ltree,'body','clause-'||n FROM generate_series(1,50) n`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(ctx, `INSERT INTO relation (id,source_document_id,target_document_id,source_key,target_key,source_number,target_number,target_provision_id,target_node_id,kind,raw_code,raw_label,evidence,method,artifact_sha256,ordinal) SELECT n,2,1,'vn:2','vn:1','n','n',11,'p11','repeals','','','evidence-'||n,'text_rule/1',repeat('b',64),n FROM generate_series(1,50) n`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(ctx, `INSERT INTO relation (id,source_document_id,target_document_id,source_key,target_key,source_number,target_number,kind,raw_code,raw_label,evidence,method,artifact_sha256,ordinal) VALUES (51,2,1,'vn:2','vn:1','n','n','cites','','','document evidence','vbpl_diagram',repeat('b',64),51)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(ctx, `INSERT INTO chunk (id,document_id,provision_id,ordinal,context_prefix,content,spans,input_sha256,token_count,embedding,sparse) SELECT 1000+n,1,1000+n,100+n,c.context_prefix,c.content,c.spans,c.input_sha256,c.token_count,c.embedding,c.sparse FROM generate_series(1,50) n CROSS JOIN chunk c WHERE c.id=1`)
	if err != nil {
		t.Fatal(err)
	}
	trace := &warningRowsTrace{}
	cfg := db.Config()
	cfg.ConnConfig.Tracer = trace
	traced, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer traced.Close()
	engine.db = traced
	arms := collectFixture(t, engine, Request{Query: "ngân hàng", Mode: "exact"}, Depths{200, 4})
	var candidates int
	for _, candidate := range arms.Primary.Members {
		if candidate.GroupID < 1001 || candidate.GroupID > 1050 {
			continue
		}
		candidates++
		if len(candidate.Relations) > 22 || len(candidate.Warnings) != 10 || candidate.WarningsOmitted != 40 {
			t.Fatalf("candidate %d: %d relations, %d warnings, %d omitted", candidate.GroupID, len(candidate.Relations), len(candidate.Warnings), candidate.WarningsOmitted)
		}
	}
	if candidates != 50 {
		t.Fatalf("hydrated %d clause candidates, want 50", candidates)
	}
	trace.queries, trace.rows = 0, 0
	tx, err := traced.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	ids := make([]int64, 50)
	for i := range ids {
		ids[i] = int64(1001 + i)
	}
	_, relations, err := hydrateTrees(ctx, tx, ids)
	if err != nil {
		t.Fatal(err)
	}
	if trace.queries != 1 || trace.rows > int64(len(ids)*22) {
		t.Fatalf("hydration returned %d relation rows across %d queries", trace.rows, trace.queries)
	}
	warnings, err := loadWarnings(ctx, tx, ids)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		rels, gaps := shapeRelations(relations[id], Recipe1(), Identity{DocumentKey: "vn:1", Anchor: fmt.Sprintf("clause-%d", id-1000)})
		if len(rels) != 20 || len(gaps) != 1 || rels[0].Kind != "cites" || rels[1].Evidence != "evidence-1" || rels[19].Evidence != "evidence-19" {
			t.Fatalf("group %d relations: %+v gaps %+v", id, rels, gaps)
		}
		if len(warnings[id].warnings) != 10 || warnings[id].omitted != 40 {
			t.Fatalf("group %d warnings: %+v", id, warnings[id])
		}
	}
}
