//go:build integration

package retrieval

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func insertDocumentRelation(t *testing.T, db *pgxpool.Pool, id int, sourceID, targetID any, sourceKey string, targetKey any, sourceNumber, targetNumber, kind, hash string, ordinal int) {
	t.Helper()
	_, err := db.Exec(context.Background(), `INSERT INTO relation (id,source_document_id,target_document_id,source_key,target_key,source_number,target_number,kind,raw_code,raw_label,evidence,method,artifact_sha256,ordinal) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'','','','',$9,$10)`, id, sourceID, targetID, sourceKey, targetKey, sourceNumber, targetNumber, kind, hash, ordinal)
	if err != nil {
		t.Fatal(err)
	}
}

func documentForTest(t *testing.T, e *Engine, key string) DocumentEntry {
	t.Helper()
	docs, err := e.Documents(context.Background(), []string{key})
	if err != nil {
		t.Fatal(err)
	}
	doc, ok := docs[key]
	if !ok {
		t.Fatalf("missing document %s", key)
	}
	return doc
}

func TestDocuments_MetadataAndCounts(t *testing.T) {
	db, e, _ := fixture(t)
	_, err := db.Exec(context.Background(), `UPDATE document SET issue_date='2019-01-02',source_url='https://example.test/doc' WHERE id=1`)
	if err != nil {
		t.Fatal(err)
	}
	h := strings.Repeat("a", 64)
	insertDocumentRelation(t, db, 1, 2, 1, "vn:2", "vn:1", "two", "one", "implements", h, 1)
	insertDocumentRelation(t, db, 2, 3, 1, "vn:3", "vn:1", "three", "one", "implements", h, 1)
	insertDocumentRelation(t, db, 3, 1, 2, "vn:1", "vn:2", "one", "two", "legal_basis", h, 1)
	d := documentForTest(t, e, "vn:1")
	if d.Number != "n" || d.Title != "title" || d.Type != "law" || d.Issuer != "issuer" || d.Issued == nil || d.Issued.String() != "2019-01-02" || d.EffectiveFrom == nil || d.EffectiveFrom.String() != "2020-01-01" || d.EffectiveTo != nil || d.SourceURL != "https://example.test/doc" {
		t.Fatalf("metadata: %+v", d)
	}
	if d.RelatedCounts["in:implements"] != 2 || d.RelatedCounts["out:legal_basis"] != 1 || len(d.RelatedCounts) != 2 || len(d.Related) != 3 {
		t.Fatalf("relations: %+v", d)
	}
	if d.Related[0].Dir != "in" || !d.Related[0].InRelease || d.Related[0].Key != "vn:3" {
		t.Fatalf("pointer: %+v", d.Related[0])
	}
}

func TestDocuments_PointerOrderTotal(t *testing.T) {
	db, e, _ := fixture(t)
	a, b := strings.Repeat("a", 64), strings.Repeat("b", 64)
	insertDocumentRelation(t, db, 1, 3, 1, "vn:z", "vn:1", "same", "one", "cites", a, 0)
	insertDocumentRelation(t, db, 2, 2, 1, "vn:a", "vn:1", "same", "one", "cites", b, 0)
	insertDocumentRelation(t, db, 3, 3, 1, "vn:a", "vn:1", "same", "one", "cites", a, 2)
	insertDocumentRelation(t, db, 4, 2, 1, "vn:a", "vn:1", "same", "one", "cites", a, 1)
	d := documentForTest(t, e, "vn:1")
	want := []string{"vn:2", "vn:3", "vn:2", "vn:3"}
	if len(d.Related) != len(want) {
		t.Fatalf("order: %+v", d.Related)
	}
	for i, p := range d.Related {
		if p.Key != want[i] {
			t.Fatalf("order: %+v", d.Related)
		}
	}
}

func TestDocuments_EndpointOutsideRelease(t *testing.T) {
	db, e, _ := fixture(t)
	insertDocumentRelation(t, db, 1, 1, nil, "vn:1", "vn:outside", "one", "other", "cites", strings.Repeat("a", 64), 1)
	p := documentForTest(t, e, "vn:1").Related[0]
	if p.Key != "vn:outside" || p.InRelease {
		t.Fatalf("pointer: %+v", p)
	}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), `"handle"`) || !strings.Contains(string(b), `"in_release":false`) {
		t.Fatalf("JSON: %s", b)
	}
}

func TestDocuments_UnresolvedEndpointNumberOnly(t *testing.T) {
	db, e, _ := fixture(t)
	insertDocumentRelation(t, db, 1, 1, nil, "vn:1", nil, "one", "other", "cites", strings.Repeat("a", 64), 1)
	p := documentForTest(t, e, "vn:1").Related[0]
	if p.Key != "" || p.Number != "other" || p.InRelease {
		t.Fatalf("pointer: %+v", p)
	}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), `"key"`) {
		t.Fatalf("JSON: %s", b)
	}
}

func TestDocuments_ManyRelationsCountsAndTopFive(t *testing.T) {
	db, e, _ := fixture(t)
	_, err := db.Exec(context.Background(), `INSERT INTO relation (id,source_document_id,target_document_id,source_key,target_key,source_number,target_number,kind,raw_code,raw_label,evidence,method,artifact_sha256,ordinal) SELECT i,2,1,'vn:2','vn:1','two','one','legal_basis','','','','',$1,i FROM generate_series(1,3000) i`, strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	insertDocumentRelation(t, db, 3001, 3, 1, "vn:3", "vn:1", "three", "one", "repeals", strings.Repeat("a", 64), 1)
	d := documentForTest(t, e, "vn:1")
	if len(d.Related) != 5 || d.Related[0].Kind != "repeals" || d.RelatedCounts["in:legal_basis"] != 3000 || d.RelatedCounts["in:repeals"] != 1 {
		t.Fatalf("relations: %+v", d)
	}
}

func TestDocuments_UnknownKeyAbsent(t *testing.T) {
	_, e, _ := fixture(t)
	docs, err := e.Documents(context.Background(), []string{"vn:1", "vn:missing"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := docs["vn:missing"]; ok || len(docs) != 1 {
		t.Fatalf("documents: %+v", docs)
	}
}

func TestDocuments_KeyIsResolvedKeyInRelease(t *testing.T) {
	db, e, _ := fixture(t)
	insertDocumentRelation(t, db, 1, 1, 2, "vn:1", nil, "one", "two", "cites", strings.Repeat("a", 64), 1)
	p := documentForTest(t, e, "vn:1").Related[0]
	if p.Key != "vn:2" || !p.InRelease {
		t.Fatalf("resolved pointer: %+v", p)
	}
}
