//go:build integration

package retrieval

import (
	"context"
	"testing"
	"time"
)

func TestOpen_Version5StillSearchesAndReads(t *testing.T) {
	db, _, embedder := fixture(t)
	ctx := context.Background()
	_, err := db.Exec(ctx, `ALTER TABLE release DROP CONSTRAINT release_schema_version_check;
 ALTER TABLE document DROP COLUMN source_withdrawn_since,DROP COLUMN source_withdrawn_rel;
 UPDATE release SET schema_version=5,rel='20990101_1'`)
	if err != nil {
		t.Fatal(err)
	}
	engine, err := Open(ctx, db, embedder, fixtureOptions())
	if err != nil {
		t.Fatal(err)
	}
	response, err := engine.Search(ctx, Request{Query: "ngân hàng", Mode: "exact"})
	if err != nil {
		t.Fatal(err)
	}
	presented, err := engine.Present(ctx, response, DetailFull)
	if err != nil {
		t.Fatal(err)
	}
	if len(presented.Documents) == 0 {
		t.Fatal("schema 5 search returned no documents")
	}
	for _, doc := range presented.Documents {
		if doc.SourceWithdrawn != nil {
			t.Fatal("schema 5 search has pin metadata")
		}
	}
	result := readItems(t, engine, ReadItem{Handle: "vn:1"})
	if result.Items[0].Document == nil || result.Items[0].Document.SourceWithdrawn != nil {
		t.Fatalf("schema 5 read=%+v", result.Items[0])
	}
}

func TestPinned_SearchAndReadDocumentEntries(t *testing.T) {
	db, engine, _ := fixture(t)
	ctx := context.Background()
	_, err := db.Exec(ctx, `UPDATE document SET source_withdrawn_since='2099-01-02',source_withdrawn_rel='20990101_1' WHERE key='vn:1'`)
	if err != nil {
		t.Fatal(err)
	}
	engine.options.Now = func() time.Time { return time.Date(2099, 1, 3, 0, 0, 0, 0, time.UTC) }
	response, err := engine.Search(ctx, Request{Query: "ngân hàng", Mode: "exact"})
	if err != nil {
		t.Fatal(err)
	}
	presented, err := engine.Present(ctx, response, DetailFull)
	if err != nil {
		t.Fatal(err)
	}
	if presented.Documents["vn:1"].SourceWithdrawn == nil {
		t.Fatal("pinned search entry lost source withdrawal")
	}
	for key, doc := range presented.Documents {
		if key != "vn:1" && doc.SourceWithdrawn != nil {
			t.Fatalf("ordinary search entry %s has withdrawal", key)
		}
	}
	result := readItems(t, engine, ReadItem{Handle: "vn:1"}, ReadItem{Handle: "vn:2"})
	if result.Items[0].Document == nil || result.Items[0].Document.SourceWithdrawn == nil || result.Items[1].Document == nil || result.Items[1].Document.SourceWithdrawn != nil {
		t.Fatalf("read=%+v", result.Items)
	}
}
