//go:build integration

package build

import (
	"context"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/source"
)

func TestLoad_PartialTreeTextCoverage(t *testing.T) {
	f := fixture(t)
	addValidityFixture(t, f, "1", "1/2024/NĐ-CP", "CHL", "2024-01-01", "2024-01-01", "Nội dung của văn bản.")
	inv := f.inventory(t)
	detail := "tree_chars=100 transcription_chars=300"
	inv.Gaps = append(inv.Gaps, InventoryGap{Stage: "extract", ItemKey: "vn:vbpl:1", Kind: "tree_text_partial", Detail: detail})
	if _, err := Load(context.Background(), f.corpus, inv, f.artifacts, f.recipes, map[string]source.StatusCode{"CHL": {Class: "in_force", Current: true}}, nil, f.asOf()); err != nil {
		t.Fatal(err)
	}
	var complete bool
	var count int
	err := f.corpus.QueryRow(context.Background(), `SELECT text_complete,(SELECT count(*) FROM coverage_gap WHERE kind='tree_text_partial' AND detail=$1 AND document_key='vn:vbpl:1') FROM document WHERE key='vn:vbpl:1'`, detail).Scan(&complete, &count)
	if err != nil || complete || count != 1 {
		t.Fatalf("complete=%v coverage=%d err=%v", complete, count, err)
	}
}
