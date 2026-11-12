//go:build integration

package retrieval

import (
	"context"
	"testing"
)

func TestNotYetEligible_PrimaryWithCurrentFalse(t *testing.T) {
	db, engine, _ := fixture(t)
	ctx := context.Background()
	statements := []string{
		`INSERT INTO document (id,country,key,number,number_norm,title,doc_type,issuer,class,source,validity_conflict,eff_from,eff_to,raw_issue_date,raw_eff_from,raw_eff_to,status_code,status_name,status_class,status_basis,is_current,is_consolidated,content_state,fetch_sha256,text_source,text_complete) SELECT 4,'vn','vn:not-yet','n','n','title','law','issuer','primary','vbpl',false,'2099-01-01',NULL,'','2099-01-01','','','','not_yet','source_status',false,false,'indexed',fetch_sha256,'source_tree',true FROM document WHERE id=2`,
		`INSERT INTO provision (id,document_id,parent_id,node_id,kind,label,heading,ordinal,preorder,path,text,anchor) VALUES (41,4,NULL,'p41','khoan','p41','',1,1,'d4'::ltree,'not yet','p41')`,
		`INSERT INTO chunk (id,document_id,provision_id,ordinal,context_prefix,content,spans,input_sha256,token_count,embedding,sparse) SELECT 6,4,41,0,context_prefix,content,spans,input_sha256,token_count,embedding,sparse FROM chunk WHERE id=4`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	engine.options.Profile = "evaluation"
	prepared, err := engine.Prepare(ctx, Request{Query: "ngân hàng", Mode: "exact"})
	if err != nil {
		t.Fatal(err)
	}
	recipe := engine.release.RetrievalRecipe
	baseline, err := engine.Collect(ctx, prepared, Depths{Dense: recipe.DenseDepth, Lexical: recipe.LexicalDepth})
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range baseline.Primary.Dense {
		if candidate.Identity.DocumentKey == "vn:not-yet" {
			t.Fatal("default eligibility returned the not-yet document")
		}
	}
	baselineResponse, err := Shape(baseline, recipe, 8, "dense")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, hit := range baselineResponse.NonCurrent {
		if hit.Identity.DocumentKey == "vn:not-yet" && !hit.Stamp.Current && hit.Stamp.StatusClass == "not_yet" {
			found = true
		}
	}
	if !found {
		t.Fatal("default tail missing not-yet document")
	}
	arms, err := engine.CollectWithNotYetEligible(ctx, prepared, Depths{Dense: recipe.DenseDepth, Lexical: recipe.LexicalDepth})
	if err != nil {
		t.Fatal(err)
	}
	response, err := Shape(arms, recipe, 8, "dense")
	if err != nil {
		t.Fatal(err)
	}
	for _, hit := range response.Hits {
		if hit.Identity.DocumentKey == "vn:not-yet" {
			if hit.Stamp.Current || hit.Stamp.StatusClass != "not_yet" {
				t.Fatalf("not-yet validity: %+v", hit.Stamp)
			}
			return
		}
	}
	t.Fatal("not-yet document missing from primary hits")
}
