//go:build integration

package build

import (
	"context"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/stage/membership"
)

func TestLoad_CorrectionEvidenceSourceGap(t *testing.T) {
	f := fixture(t)
	addValidityFixture(t, f, "1", "1/2024/NĐ-CP", "CHL", "2024-01-01", "2024-01-01", "")
	f.fetch(t, "vn:vbpl:2")
	inv := f.inventory(t)
	inv.Version = 2
	inv.Membership.Entries = []membership.Entry{{DocumentKey: "vn:vbpl:1", Disposition: "active", Class: "primary", Source: "vbpl"}, {DocumentKey: "vn:vbpl:2", Disposition: "active", Class: "relation_context", Source: "vbpl"}}
	for i := range inv.Items {
		if inv.Items[i].Stage == "fetch" && inv.Items[i].ItemKey == "vn:vbpl:2" {
			inv.Items[i].State = "gap"
		}
	}
	inv.Gaps = append(inv.Gaps, InventoryGap{Stage: "fetch", ItemKey: "vn:vbpl:2", Kind: "source_unavailable"})
	c := correctionFixture()
	c.Evidence.Key = "vn:vbpl:2"
	counts, err := Load(context.Background(), f.corpus, inv, f.artifacts, f.recipes, map[string]source.StatusCode{"CHL": {Class: "in_force", Current: true}}, []source.Correction{c}, f.asOf())
	if err != nil {
		t.Fatal(err)
	}
	var conflict bool
	var class string
	var decisions, gaps int
	err = f.corpus.QueryRow(context.Background(), `SELECT validity_conflict,status_class,(SELECT count(*) FROM validity_decision WHERE rule_version='correction_unverified'),(SELECT count(*) FROM coverage_gap WHERE kind='correction_evidence_unavailable' AND accepted_at IS NULL) FROM document WHERE key='vn:vbpl:1'`).Scan(&conflict, &class, &decisions, &gaps)
	if err != nil || !conflict || class != "in_force" || decisions != 1 || gaps != 1 {
		t.Fatalf("conflict=%v class=%s decisions=%d gaps=%d err=%v", conflict, class, decisions, gaps, err)
	}
	review, err := encodeValidityReview(counts.validity.reviews)
	if err != nil || !strings.Contains(string(review), "correction_evidence_unavailable") {
		t.Fatalf("review=%s err=%v", review, err)
	}
}
