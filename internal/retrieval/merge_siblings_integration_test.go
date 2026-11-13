//go:build integration

package retrieval

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestSiblingMerge_HydratesArticleAttachments(t *testing.T) {
	db, engine, _ := fixture(t)
	ctx := context.Background()
	statements := []string{
		`INSERT INTO provision (id,document_id,parent_id,node_id,kind,label,heading,ordinal,preorder,path,text,anchor) VALUES (14,1,11,'p14','khoan','Khoản 2','',2,4,'d1.p3'::ltree,'sibling text','p14')`,
		`INSERT INTO relation (id,source_document_id,source_key,source_number,target_document_id,target_key,target_number,kind,raw_code,raw_label,target_provision_id,target_node_id,evidence,method,artifact_sha256,ordinal) VALUES (1,2,'vn:2','actor',1,'vn:1','n','repeals','','',14,'p14','sibling warning','text_rule/1',repeat('b',64),0)`,
	}
	for _, sql := range statements {
		if _, err := db.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	collected := collectFixture(t, engine, Request{Query: "ngân hàng", Mode: "exact"}, Depths{50, 50})
	arms := collected
	arms.Primary = ArmGroup{}
	arms.NonCurrent = ArmGroup{}
	for _, c := range collected.Primary.Members {
		if c.GroupID == 12 {
			arms.Primary.Members = append(arms.Primary.Members, c)
			if len(arms.Primary.Dense) == 0 {
				arms.Primary.Dense = append(arms.Primary.Dense, c)
			}
		}
	}
	if len(arms.Primary.Dense) == 0 {
		t.Fatal("missing clause fixture")
	}
	before := arms.Primary.Members[0]
	engine.options.Profile = "evaluation"
	merge, err := engine.HydrateSiblingMerge(ctx, arms)
	if err != nil {
		t.Fatal(err)
	}
	article, ok := merge.Articles[11]
	if !ok || len(article.Descendants) != 4 || len(article.Relations) != 1 || len(article.Warnings) != 1 || !strings.HasSuffix(article.Warnings[0].Target, "#p14") {
		t.Fatalf("article metadata %+v", article)
	}
	if !reflect.DeepEqual(before, arms.Primary.Members[0]) {
		t.Fatal("hydration changed candidate")
	}
	if len(before.Relations) != 0 || len(before.Warnings) != 0 {
		t.Fatal("clause unexpectedly caches sibling attachments")
	}
	sibling := article.Descendants[3]
	c := before
	c.ChunkID, c.ProvisionID, c.GroupID = 100, sibling.ID, sibling.ID
	c.Identity, c.Group, c.Provision = sibling.Identity, sibling.Identity, sibling
	c.Ancestors = append(append([]Provision(nil), article.Ancestors...), sibling)
	c.Descendants = []Provision{sibling}
	c.Spans = nil
	arms.Primary.Dense = append(arms.Primary.Dense, c)
	arms.Primary.Members = append(arms.Primary.Members, c)
	recipe := engine.release.RetrievalRecipe
	recipe.NonCurrentTail = 0
	response, err := merge.Shape(arms, recipe, 8, "dense", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Hits) != 1 || response.Hits[0].Identity != article.Identity || len(response.Hits[0].Warnings) != 1 || len(response.Hits[0].Relations) != 1 {
		t.Fatalf("merged response %+v", response)
	}
}
