package build

import (
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/stage/membership"
)

func gapCorrection() source.Correction {
	status := "expired"
	return source.Correction{Key: "a", Number: "A", Set: source.CorrectionSet{StatusClass: &status}, Reason: "source error", Evidence: source.CorrectionEvidence{Key: "b", Anchor: "article", Quote: "quote"}, Reviewed: "2099-01-01"}
}
func gapLoader() loader {
	return loader{members: map[string]membership.Entry{"a": {DocumentKey: "a", Disposition: "active"}, "b": {DocumentKey: "b", Disposition: "active"}}, gaps: []InventoryGap{{Stage: "fetch", ItemKey: "b", Kind: "source_unavailable"}}}
}
func TestCorrections_EvidenceDocumentSourceGapHoldsEntry(t *testing.T) {
	l := gapLoader()
	docs := map[string]correctionDocument{"a": {number: "A"}}
	entries, held, err := l.validateCorrectionGaps([]source.Correction{gapCorrection()}, docs)
	if err != nil || len(entries) != 0 || len(held) != 1 {
		t.Fatalf("entries=%v held=%v err=%v", entries, held, err)
	}
	d := testDoc("a", "A", "2099-01-01", "2099-01-01")
	result, err := runValidity([]validityDocument{d}, nil, entries, "2099-01-01")
	if err != nil {
		t.Fatal(err)
	}
	if err = l.recordCorrectionGaps(&result, held); err != nil {
		t.Fatal(err)
	}
	out := result.documents["a"]
	if !out.values.Conflict || out.values.Class != "in_force" || out.history[len(out.history)-1].version != "correction_unverified" || !hasReview(result, "correction_evidence_unavailable") {
		t.Fatalf("result=%+v", result)
	}
	if len(l.gaps) != 2 || !strings.Contains(l.gaps[1].Detail, "source_unavailable") {
		t.Fatalf("gaps=%+v", l.gaps)
	}
}
func TestCorrections_TargetSourceGapSkipsEntry(t *testing.T) {
	l := gapLoader()
	l.gaps = []InventoryGap{{Stage: "fetch", ItemKey: "a", Kind: "source_unavailable"}}
	entries, held, err := l.validateCorrectionGaps([]source.Correction{gapCorrection()}, map[string]correctionDocument{"b": {number: "B", provisions: map[string]string{"article": "quote"}}})
	if err != nil || len(entries) != 0 || len(held) != 1 {
		t.Fatalf("entries=%v held=%v err=%v", entries, held, err)
	}
	result, err := runValidity(nil, nil, entries, "2099-01-01")
	if err != nil {
		t.Fatal(err)
	}
	if err = l.recordCorrectionGaps(&result, held); err != nil || len(result.reviews) != 1 || len(l.gaps) != 2 {
		t.Fatalf("result=%+v gaps=%+v err=%v", result, l.gaps, err)
	}
}
func TestCorrections_OtherGapKindsStillFail(t *testing.T) {
	for _, kind := range []string{"scope_removed", "missing_attachment", "not_found"} {
		for _, missing := range []string{"a", "b"} {
			t.Run(kind+"/"+missing, func(t *testing.T) {
				l := gapLoader()
				l.gaps = []InventoryGap{{Stage: "fetch", ItemKey: missing, Kind: kind}}
				docs := map[string]correctionDocument{"a": {number: "A"}, "b": {number: "B", provisions: map[string]string{"article": "quote"}}}
				delete(docs, missing)
				if _, _, err := l.validateCorrectionGaps([]source.Correction{gapCorrection()}, docs); err == nil {
					t.Fatal("accepted missing document")
				}
			})
		}
	}
}

func TestCorrections_MissingAnchorWithTextGapFails(t *testing.T) {
	l := gapLoader()
	l.gaps = []InventoryGap{{Stage: "extract", ItemKey: "b", Kind: "needs_ocr"}}
	if _, _, err := l.validateCorrectionGaps([]source.Correction{gapCorrection()}, map[string]correctionDocument{"a": {number: "A"}, "b": {number: "B"}}); err == nil {
		t.Fatal("accepted missing anchor")
	}
}

func TestCorrections_AbsentFromMembershipStillFails(t *testing.T) {
	l := gapLoader()
	delete(l.members, "b")
	if _, _, err := l.validateCorrectionGaps([]source.Correction{gapCorrection()}, map[string]correctionDocument{"a": {number: "A"}}); err == nil {
		t.Fatal("accepted missing membership")
	}
}
func TestCorrections_QuoteMismatchStillFails(t *testing.T) {
	l := gapLoader()
	l.gaps = []InventoryGap{{Stage: "extract", ItemKey: "b", Kind: "needs_ocr"}}
	if _, _, err := l.validateCorrectionGaps([]source.Correction{gapCorrection()}, map[string]correctionDocument{"a": {number: "A"}, "b": {number: "B", provisions: map[string]string{"article": "other"}}}); err == nil {
		t.Fatal("accepted quote mismatch")
	}
}

func TestCorrections_OwnAnchorMissingWithTextGapFails(t *testing.T) {
	l := gapLoader()
	l.gaps = []InventoryGap{{Stage: "extract", ItemKey: "a", Kind: "needs_ocr"}}
	c := gapCorrection()
	c.Evidence.Key = ""
	if _, _, err := l.validateCorrectionGaps([]source.Correction{c}, map[string]correctionDocument{"a": {number: "A"}}); err == nil {
		t.Fatal("accepted missing anchor")
	}
}
