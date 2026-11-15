//go:build integration

package retrieval

import (
	"context"
	"reflect"
	"testing"
)

func TestWarnings_ReviewedMatchesText(t *testing.T) {
	db, engine, _ := fixture(t)
	ctx := context.Background()
	_, err := db.Exec(ctx, `INSERT INTO relation(id,source_document_id,target_document_id,source_provision_id,target_provision_id,source_key,target_key,source_number,target_number,kind,raw_code,raw_label,target_node_id,evidence,method,artifact_sha256,ordinal) VALUES
 (1,2,1,21,11,'vn:2','vn:1','n','n','repeals','','','p11','exact','text_rule/1',repeat('a',64),1),
 (2,2,1,21,11,'vn:2','vn:1','n','n','suspends','','','p11/missing','inexact','text_rule/1',repeat('a',64),2),
 (3,2,1,21,NULL,'vn:2','vn:1','n','n','replaces','','','p11','unresolved','text_rule/1',repeat('a',64),3),
 (4,2,1,21,12,'vn:2','vn:1','n','n','amends_supplements','','','p12','target','text_rule/1',repeat('a',64),4)`)
	if err != nil {
		t.Fatal(err)
	}
	type result struct {
		hitWarnings, readWarnings []Warning
		targetWarnings            []Warning
		hitRelations              []Relation
		readRelations             []Unit
	}
	collect := func() result {
		response, err := engine.Search(ctx, Request{Query: "ngân hàng khoản", Mode: "exact", Arms: "lexical", Limit: 20})
		if err != nil {
			t.Fatal(err)
		}
		presented, err := engine.Present(ctx, response, DetailFull)
		if err != nil {
			t.Fatal(err)
		}
		var got result
		for _, hit := range presented.Hits {
			if hit.Handle == "vn:1#p12" {
				got.hitWarnings = hit.Warnings
				got.hitRelations = hit.Relations
			}
		}
		read := readItems(t, engine, ReadItem{Handle: "vn:1#p12"}, ReadItem{Handle: "vn:1#p12", Relations: true}, ReadItem{Handle: "vn:1#p11"})
		for _, unit := range read.Items[0].Units {
			if unit.Anchor == "p12" {
				got.readWarnings = unit.Warnings
			}
		}
		got.readRelations = read.Items[1].Units
		for _, unit := range read.Items[2].Units {
			if unit.Anchor == "p11" {
				got.targetWarnings = unit.Warnings
			}
		}
		if len(got.hitWarnings) != 2 || len(got.readWarnings) != 2 || len(got.targetWarnings) != 3 || len(got.hitRelations) == 0 || len(got.readRelations) == 0 {
			t.Fatalf("missing inherited warning or relation: %+v", got)
		}
		return got
	}
	want := collect()
	for _, method := range []string{"reviewed", "reviewed,vbpl_diagram"} {
		_, err = db.Exec(ctx, `UPDATE relation SET method=$1`, method)
		if err != nil {
			t.Fatal(err)
		}
		got := collect()
		for i := range got.hitRelations {
			got.hitRelations[i].Method = "text_rule/1"
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: got %+v, want %+v", method, got, want)
		}
	}
}
