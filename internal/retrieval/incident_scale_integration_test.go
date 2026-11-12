//go:build integration

package retrieval

import (
	"context"
	"testing"
)

func TestHydrate_IncidentRelationsBoundedAndWarningsComplete(t *testing.T) {
	db, engine, _ := fixture(t)
	ctx := context.Background()
	_, err := db.Exec(ctx, `INSERT INTO relation (id,source_document_id,target_document_id,source_key,target_key,source_number,target_number,kind,raw_code,raw_label,evidence,method,artifact_sha256,ordinal) SELECT n,2,1,'vn:2','vn:1','n','n','cites','','','evidence-'||n,'vbpl_diagram',repeat('b',64),n FROM generate_series(1,30) n`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(ctx, `INSERT INTO relation (id,source_document_id,target_document_id,target_provision_id,source_key,target_key,source_number,target_number,kind,raw_code,raw_label,target_node_id,evidence,method,artifact_sha256,ordinal) SELECT 30+n,2,1,11,'vn:2','vn:1','n','n','repeals','','','p11','warning-'||n,'text_rule/1',repeat('b',64),30+n FROM generate_series(1,15) n`)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	_, relations, err := hydrateTrees(ctx, tx, []int64{11, 12})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{11, 12} {
		// The clause also inherits at most 21 warnings from its article.
		maxRows := 21
		if id == 12 {
			maxRows += 15
		}
		if len(relations[id]) > maxRows {
			t.Fatalf("group %d fetched %d relations, want at most %d", id, len(relations[id]), maxRows)
		}
	}
	recipe := engine.Release().RetrievalRecipe
	arms := collectFixture(t, engine, Request{Query: "ngân hàng", Mode: "exact"}, Depths{recipe.DenseDepth, recipe.LexicalDepth})
	response, err := Shape(arms, recipe, 8, "hybrid")
	if err != nil {
		t.Fatal(err)
	}
	for _, hit := range response.Hits {
		if hit.Identity.DocumentKey != "vn:1" || hit.Identity.Anchor != "p11" {
			continue
		}
		if len(hit.Warnings) != 10 || hit.WarningsOmitted != 5 {
			t.Fatalf("hit warnings: %+v omitted %d", hit.Warnings, hit.WarningsOmitted)
		}
		if len(hit.Relations) != 20 {
			t.Fatalf("hit has %d relations", len(hit.Relations))
		}
		for _, relation := range hit.Relations {
			if relation.Kind != "cites" {
				t.Fatalf("unexpected relation: %+v", relation)
			}
		}
		if hit.Relations[0].Evidence != "evidence-1" || hit.Relations[19].Evidence != "evidence-20" {
			t.Fatalf("relation order: %+v", hit.Relations)
		}
		for _, gap := range hit.Gaps {
			if gap.Kind == "relations_omitted" {
				return
			}
		}
		t.Fatal("missing relations_omitted gap")
	}
	t.Fatal("missing article hit")
}
