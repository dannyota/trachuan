//go:build integration

package retrieval

import (
	"context"
	"strings"
	"testing"
)

func TestPresent_LoadsDocumentsForAllHits(t *testing.T) {
	_, e, _ := fixture(t)
	a, b := presentationHit("vn:1", "p11"), presentationHit("vn:2", "p21")
	a.Stamp = Validity{StatusClass: "in_force", Current: true}
	b.Stamp = Validity{StatusClass: "expired"}
	got, err := e.Present(context.Background(), Response{Hits: []Hit{a, a}, NonCurrent: []Hit{b}}, DetailCompact)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Documents) != 2 || got.Documents["vn:1"].Title != "title" || !got.Documents["vn:1"].Status.Current || got.Documents["vn:2"].Status.Class != "expired" || len(got.Hits) != 2 || len(got.NonCurrent) != 1 {
		t.Fatalf("result: %+v", got)
	}
}

func TestPresent_HydratesIntraDocumentEndpoints(t *testing.T) {
	db, _, _ := fixture(t)
	ctx := context.Background()
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	_, err = tx.Exec(ctx, `INSERT INTO provision (id,document_id,node_id,kind,label,heading,ordinal,preorder,path,text,anchor) VALUES (14,1,'other','dieu','other','',2,4,'d1.other'::ltree,'other','unrelated-anchor')`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO relation (id,source_document_id,target_document_id,source_provision_id,target_provision_id,source_key,target_key,source_number,target_number,kind,raw_code,raw_label,evidence,method,artifact_sha256,ordinal) VALUES (1,1,1,12,14,'vn:1','vn:1','n','n','repeals','','','','',$1,0)`, strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO relation (id,source_document_id,target_document_id,source_key,target_key,source_number,target_number,kind,raw_code,raw_label,evidence,method,artifact_sha256,ordinal) VALUES (2,1,1,'vn:1','vn:1','n','n','cites','','','','',$1,1)`, strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	_, rels, err := hydrateTrees(ctx, tx, []int64{11, 14})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		id             int64
		source, target bool
	}{{11, true, false}, {14, false, true}} {
		if len(rels[tc.id]) != 2 {
			t.Fatalf("group %d relations: %+v", tc.id, rels[tc.id])
		}
		shaped, _ := shapeRelations(rels[tc.id], Recipe2(), Identity{})
		if len(shaped) != 2 || shaped[0].Kind != "cites" || shaped[0].SourceInHit || shaped[0].TargetInHit || shaped[1].Kind != "repeals" || shaped[1].SourceInHit != tc.source || shaped[1].TargetInHit != tc.target {
			t.Fatalf("group %d relations: %+v", tc.id, shaped)
		}
	}
}
