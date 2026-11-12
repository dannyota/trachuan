//go:build integration

package retrieval

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestWarnings_AttachmentAndRecovery(t *testing.T) {
	db, engine, _ := fixture(t)
	ctx := context.Background()
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	for i := 0; i < 25; i++ {
		target, named := 11, "p11"
		if i%2 == 0 {
			target, named = 12, "p12"
		}
		_, err = tx.Exec(ctx, `INSERT INTO relation(id,source_document_id,target_document_id,source_provision_id,target_provision_id,source_key,target_key,source_number,target_number,kind,raw_code,raw_label,target_node_id,evidence,method,artifact_sha256,ordinal) VALUES($1::bigint,2,1,21,$2,'vn:2','vn:1','n','n','repeals','','',$3,$4,'text_rule/1',$5,($1::bigint)::integer)`, i+1, target, named, fmt.Sprintf("evidence-%d", i), strings.Repeat("a", 64))
		if err != nil {
			t.Fatal(err)
		}
	}
	_, err = tx.Exec(ctx, `INSERT INTO relation(id,source_document_id,target_document_id,source_key,target_key,source_number,target_number,kind,raw_code,raw_label,target_provision_id,target_node_id,evidence,method,artifact_sha256,ordinal) VALUES(26,2,1,'vn:2','vn:1','n','n','suspends','','',11,'p11/missing','inexact','text_rule/1',$1,26)`, strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	bounded, err := loadWarnings(ctx, tx, []int64{11, 12, 13})
	if err != nil {
		t.Fatal(err)
	}
	article, omitted := bounded[11].warnings, bounded[11].omitted
	if len(article) != 10 || omitted != 16 || article[0].Target != "vn:1#p12" || article[0].Citation != "p11, p12" {
		t.Fatalf("article warnings %+v omitted %d", article, omitted)
	}
	hit := presentationHit("vn:1", "p11")
	hit.Warnings = article
	presented := present(Response{Hits: []Hit{hit}}, nil, DetailCompact).Hits[0]
	if presented.Warnings[0].Target != "vn:1#p12" || presented.Warnings[0].Citation != "p11, p12" {
		t.Fatalf("article hit warnings %+v", presented.Warnings)
	}
	clause, omitted := bounded[12].warnings, bounded[12].omitted
	if len(clause) != 10 || omitted != 15 {
		t.Fatalf("clause warnings %+v omitted %d", clause, omitted)
	}
	point, omitted := bounded[13].warnings, bounded[13].omitted
	if len(point) != 10 || omitted != 15 {
		t.Fatalf("point warnings %+v omitted %d", point, omitted)
	}
	nodes, err := readNodes(ctx, tx, 1, "vn:1")
	if err != nil {
		t.Fatal(err)
	}
	units := []Unit{{Kind: "provision", Anchor: "p12"}}
	if err = attachReadWarnings(ctx, tx, nodes, units); err != nil {
		t.Fatal(err)
	}
	if len(units[0].Warnings) != 10 || units[0].WarningsOmitted != 15 {
		t.Fatalf("read unit %+v", units[0])
	}
	var all []Unit
	for offset := 0; ; {
		seq, readErr := readRelations(ctx, tx, "vn:1", []int64{12, 13}, []int64{11, 12}, true, offset, 1000)
		if readErr != nil {
			t.Fatal(readErr)
		}
		all = append(all, seq.units...)
		if !seq.more {
			break
		}
		offset += len(seq.units)
	}
	if len(all) != 25 {
		t.Fatalf("recovered %d relations, want 25", len(all))
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	result := readItems(t, engine, ReadItem{Handle: "vn:1#p12"}, ReadItem{Handle: "vn:1#p12", Relations: true})
	found := false
	for _, unit := range result.Items[0].Units {
		if unit.Anchor == "p12" {
			found = true
			if len(unit.Warnings) != 10 || unit.WarningsOmitted != 15 {
				t.Fatalf("read unit: %+v", unit)
			}
		}
	}
	if !found {
		t.Fatal("missing clause unit")
	}
	count, next := len(result.Items[1].Units), result.Items[1].Next
	for next != "" {
		page := readItems(t, engine, ReadItem{Next: next})
		count += len(page.Items[0].Units)
		next = page.Items[0].Next
	}
	if count != 25 {
		t.Fatalf("recovered %d paged relations, want 25", count)
	}
}
