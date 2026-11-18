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

func TestWarnings_ReviewedDocumentLevelMatchesDiagram(t *testing.T) {
	db, engine, _ := fixture(t)
	ctx := context.Background()
	_, execErr := db.Exec(ctx, `INSERT INTO relation(id,source_document_id,target_document_id,source_key,target_key,source_number,target_number,kind,raw_code,raw_label,evidence,method,artifact_sha256,ordinal) VALUES
 (1,2,1,'vn:2','vn:1','n','n','repeals','','bãi bỏ','quoted operative sentence','vbpl_diagram',repeat('a',64),1)`)
	if execErr != nil {
		t.Fatal(execErr)
	}
	type surface struct {
		reads        []ItemResult
		hitRelations []Relation
		hitWarnings  []Warning
	}
	handles := []ReadItem{{Handle: "vn:1#p11", Relations: true}, {Handle: "vn:1", Relations: true}, {Handle: "vn:2", Relations: true}, {Handle: "vn:2#p21", Relations: true}, {Handle: "vn:1"}, {Handle: "vn:1#p11"}}
	collect := func(method string) surface {
		var got surface
		got.reads = readItems(t, engine, handles...).Items
		for i, item := range got.reads {
			if item.Error != "" {
				t.Fatalf("%s: read %s: %s", method, handles[i].Handle, item.Error)
			}
		}
		response, err := engine.Search(ctx, Request{Query: "ngân hàng khoản", Mode: "exact", Arms: "lexical", Limit: 20})
		if err != nil {
			t.Fatal(err)
		}
		presented, err := engine.Present(ctx, response, DetailFull)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, hit := range presented.Hits {
			if hit.Handle == "vn:1#p12" {
				found = true
				got.hitRelations = hit.Relations
				got.hitWarnings = hit.Warnings
			}
		}
		if !found {
			t.Fatalf("%s: hit vn:1#p12 missing", method)
		}
		for i := range got.hitRelations {
			if got.hitRelations[i].SourceKey != "vn:2" {
				continue
			}
			if got.hitRelations[i].Method != method {
				t.Fatalf("hit relation method = %q, want %q", got.hitRelations[i].Method, method)
			}
			got.hitRelations[i].Method = "normalized"
		}
		return got
	}
	want := collect("vbpl_diagram")
	provision := false
	for _, unit := range want.reads[5].Units {
		if unit.Anchor == "p11" {
			provision = true
			if len(unit.Warnings) != 0 {
				t.Fatalf("diagram row warned on vn:1#p11: %+v", unit.Warnings)
			}
		}
	}
	if !provision {
		t.Fatalf("vn:1#p11 unit missing: %+v", want.reads[5].Units)
	}
	in := 0
	for _, unit := range want.reads[0].Units {
		if r := unit.Relation; r != nil && r.Dir == "in" && r.Key == "vn:2" && r.InRelease {
			in++
		}
	}
	if in != 1 {
		t.Fatalf("relation units from vn:2 = %d, want 1: %+v", in, want.reads[0].Units)
	}
	if len(want.hitWarnings) != 0 {
		t.Fatalf("diagram row warned: %+v", want.hitWarnings)
	}
	if _, execErr = db.Exec(ctx, `UPDATE relation SET method='reviewed'`); execErr != nil {
		t.Fatal(execErr)
	}
	got := collect("reviewed")
	for i := range handles {
		if !reflect.DeepEqual(got.reads[i], want.reads[i]) {
			t.Errorf("read %s: got %+v, want %+v", handles[i].Handle, got.reads[i], want.reads[i])
		}
	}
	if !reflect.DeepEqual(got.hitRelations, want.hitRelations) || !reflect.DeepEqual(got.hitWarnings, want.hitWarnings) {
		t.Errorf("hit: got %+v %+v, want %+v %+v", got.hitRelations, got.hitWarnings, want.hitRelations, want.hitWarnings)
	}
}
