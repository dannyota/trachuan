//go:build integration

package build

import (
	"context"
	"fmt"
	"testing"
)

func TestLoad_RelationNumberResolutionCountsExcluded(t *testing.T) {
	f := fixture(t)
	fetches := map[int]string{}
	for _, doc := range []struct {
		id     int
		number string
	}{{1, "source"}, {2, "01/2024/TT"}, {3, "01/2024/TT"}, {4, "02/2024/TT"}, {5, "03/2024/TT"}} {
		key := fmt.Sprintf("vn:vbpl:%d", doc.id)
		a := f.addArtifact(t, "fetch", key, map[string]any{"detail.json": map[string]any{"data": map[string]any{"id": fmt.Sprint(doc.id), "docNum": doc.number}}, "files.json": []any{}})
		fetches[doc.id] = a.SHA256
	}
	if _, err := f.workshop.Exec(context.Background(), `INSERT INTO gap(run_id,stage,item_key,kind,detail) VALUES($1,'fetch','vn:vbpl:3','withdrawn',''),($1,'fetch','vn:vbpl:5','withdrawn','')`, f.run.ID); err != nil {
		t.Fatal(err)
	}
	f.addArtifact(t, "relate", "vn:vbpl:1", map[string]any{"relations.json": []map[string]any{{"ordinal": 0, "kind": "references", "target_number": "01/2024/TT"}, {"ordinal": 1, "kind": "references", "target_number": "02/2024/TT"}, {"ordinal": 2, "kind": "references", "target_number": "03/2024/TT"}}}, fetches[1])
	if _, err := f.load(t); err != nil {
		t.Fatal(err)
	}
	var key *string
	if err := f.corpus.QueryRow(context.Background(), `SELECT target_key FROM relation WHERE ordinal=0`).Scan(&key); err != nil || key != nil {
		t.Fatalf("ambiguous target = %v, %v", key, err)
	}
	var targetID *int64
	if err := f.corpus.QueryRow(context.Background(), `SELECT target_document_id FROM relation WHERE ordinal=0`).Scan(&targetID); err != nil || targetID != nil {
		t.Fatalf("ambiguous target id = %v, %v", targetID, err)
	}
	if err := f.corpus.QueryRow(context.Background(), `SELECT target_key FROM relation WHERE ordinal=1`).Scan(&key); err != nil || key == nil || *key != "vn:vbpl:4" {
		t.Fatalf("unique target = %v, %v", key, err)
	}
	if err := f.corpus.QueryRow(context.Background(), `SELECT target_key,target_document_id FROM relation WHERE ordinal=2`).Scan(&key, &targetID); err != nil || key == nil || *key != "vn:vbpl:5" || targetID != nil {
		t.Fatalf("excluded unique target = %v, id %v, %v", key, targetID, err)
	}
}
