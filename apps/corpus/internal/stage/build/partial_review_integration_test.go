//go:build integration

package build

import (
	"context"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/stage/membership"
)

func TestLoad_PartialReviewAndGaps(t *testing.T) {
	f := fixture(t)
	addValidityFixture(t, f, "1", "1", "HHL1P", "2020-01-01", "2020-01-01", "")
	addValidityFixture(t, f, "2", "2", "HHL1P", "2020-01-01", "2020-01-01", "")
	hash := addValidityFixture(t, f, "3", "3", "CHL", "2020-01-01", "2020-01-01", "")
	f.addArtifact(t, "relate", "vn:vbpl:3", map[string]any{"relations.json": []map[string]any{{"ordinal": 0, "kind": "amends_supplements", "target_vbpl_id": "1"}}}, hash)
	inv := f.inventory(t)
	inv.Version = 2
	inv.Membership.Entries = []membership.Entry{{DocumentKey: "vn:vbpl:1", Disposition: "active", Class: "relation_context", Source: "vbpl"}, {DocumentKey: "vn:vbpl:2", Disposition: "active", Class: "primary", Source: "vbpl"}, {DocumentKey: "vn:vbpl:3", Disposition: "active", Class: "primary", Source: "vbpl"}}
	counts, err := Load(context.Background(), f.corpus, inv, f.artifacts, f.recipes, map[string]source.StatusCode{"HHL1P": {Class: "partial", Current: true}, "CHL": {Class: "in_force", Current: true}}, nil, f.asOf())
	if err != nil {
		t.Fatal(err)
	}
	partial := counts.validity.partialDocuments
	if len(partial) != 2 || partial[0].Key != "vn:vbpl:1" || partial[0].Class != "relation_context" || len(partial[0].Acting) != 1 || partial[0].Acting[0] != (partialActor{Key: "vn:vbpl:3", Number: "3", Kind: "amends_supplements"}) || partial[1].Key != "vn:vbpl:2" || len(partial[1].Acting) != 0 {
		t.Fatalf("partial = %#v", partial)
	}
	var key, detail string
	var count int
	err = f.corpus.QueryRow(context.Background(), `SELECT count(*), coalesce(min(item_key),''), coalesce(min(detail),'') FROM coverage_gap WHERE stage='build' AND kind='partial_status_unexplained' AND accepted_at IS NULL`).Scan(&count, &key, &detail)
	if err != nil || count != 1 || key != "vn:vbpl:2" || detail != "HHL1P" {
		t.Fatalf("gaps %d %q %q: %v", count, key, detail, err)
	}
}
