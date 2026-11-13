//go:build integration

package retrieval

import (
	"context"
	"strings"
	"testing"
)

func TestWarnings_ClauseOwnedHitTargetsAndStatus(t *testing.T) {
	db, engine, _ := fixture(t)
	ctx := context.Background()
	_, err := db.Exec(ctx, `INSERT INTO provision(id,document_id,parent_id,node_id,kind,label,heading,ordinal,preorder,path,text,anchor) VALUES(14,1,11,'sibling','khoan','Khoản 2','',2,4,'d1.sibling','Sibling text','sibling')`)
	if err != nil {
		t.Fatal(err)
	}
	// Article, self and descendant warnings attach. Siblings and an inexact
	// article warning must not change the clause's stamp or warnings.
	for i, tc := range []struct {
		target          int
		named, evidence string
	}{
		{11, "p11", "exact article"}, {12, "p12", "exact clause"}, {13, "p13", "inside clause"}, {14, "sibling", "sibling clause"}, {11, "p11/missing", "inexact article"},
	} {
		_, err = db.Exec(ctx, `INSERT INTO relation(id,source_document_id,target_document_id,source_provision_id,target_provision_id,source_key,target_key,source_number,target_number,kind,raw_code,raw_label,target_node_id,evidence,method,artifact_sha256,ordinal) VALUES($1::bigint,2,1,21,$2,'vn:2','vn:1','n','n','suspends','','',$3,$4,'text_rule/1',$5,($1::bigint)::integer)`, i+1, tc.target, tc.named, tc.evidence, strings.Repeat("a", 64))
		if err != nil {
			t.Fatal(err)
		}
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	sets, err := loadWarnings(ctx, tx, []int64{12})
	if err != nil {
		t.Fatal(err)
	}
	got := sets[12]
	if len(got.warnings) != 3 || got.omitted != 0 {
		t.Fatalf("clause warnings=%+v", got)
	}
	targets := map[string]bool{}
	for _, w := range got.warnings {
		targets[w.Target] = true
	}
	for _, target := range []string{"vn:1#p11", "vn:1#p12", "vn:1#p13"} {
		if !targets[target] {
			t.Errorf("missing %s: %+v", target, got)
		}
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	response, err := engine.Search(ctx, Request{Query: "ngân hàng khoản", Mode: "exact", Arms: "lexical", Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	presented, err := engine.Present(ctx, response, DetailFull)
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range presented.Hits {
		if h.Handle != "vn:1#p12" {
			continue
		}
		if len(h.Warnings) != 3 || presented.Documents[h.Doc].Status.Class != "in_force" || !presented.Documents[h.Doc].Status.Current {
			t.Fatalf("hit=%+v status=%+v", h, presented.Documents[h.Doc].Status)
		}
		return
	}
	t.Fatalf("missing clause-owned hit: %+v", presented.Hits)
}
