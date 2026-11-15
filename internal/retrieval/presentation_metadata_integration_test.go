//go:build integration

package retrieval

import (
	"context"
	"strings"
	"testing"
)

func TestPresentation_DocumentMetadataWhitespace(t *testing.T) {
	db, engine, _ := fixture(t)
	ctx := context.Background()
	raw := "\n  Alpha\u00a0 \t Beta  "
	legal := "ngân hàng  \n khoản"
	readExec(t, db, `UPDATE document SET number=$1,title=$1 WHERE id=1`, raw)
	readExec(t, db, `UPDATE provision SET text=$1 WHERE id=13`, legal)
	readExec(t, db, `INSERT INTO relation (id,source_document_id,target_document_id,source_key,target_key,source_number,target_number,kind,raw_code,raw_label,evidence,method,artifact_sha256,ordinal) VALUES (1,1,2,'vn:1','vn:2',$1,$1,'cites','',$2,$2,'source',$3,0)`, raw, legal, strings.Repeat("b", 64))
	response, err := engine.Search(ctx, Request{Query: "ngân hàng", Mode: "exact"})
	if err != nil {
		t.Fatal(err)
	}
	for _, detail := range []Detail{DetailCompact, DetailFull} {
		result, presentErr := engine.Present(ctx, response, detail)
		if presentErr != nil {
			t.Fatal(presentErr)
		}
		doc, ok := result.Documents["vn:1"]
		if !ok || doc.Number != "Alpha Beta" || doc.Title != "Alpha Beta" || len(doc.Related) != 1 || doc.Related[0].Number != "Alpha Beta" {
			t.Fatalf("%s document: %+v", detail, doc)
		}
	}
	result := readItems(t, engine, ReadItem{Handle: "vn:1"}, ReadItem{Handle: "vn:1#p13"}, ReadItem{Handle: "vn:1", Relations: true})
	doc := result.Items[0].Document
	if doc == nil || doc.Number != "Alpha Beta" || doc.Title != "Alpha Beta" || len(doc.Related) != 1 || doc.Related[0].Number != "Alpha Beta" {
		t.Fatalf("read document: %+v", doc)
	}
	units := result.Items[1].Units
	if len(units) == 0 || units[len(units)-1].Text != legal {
		t.Fatalf("read text: %+v", units)
	}
	relations := result.Items[2].Units
	if len(relations) != 1 || relations[0].Relation.Number != "Alpha Beta" || relations[0].Relation.Evidence != legal {
		t.Fatalf("read relations: %+v", relations)
	}
}
