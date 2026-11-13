package build

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/textparse"
)

func TestCorrections_RecordedTranscriptionEvidenceFailsClosed(t *testing.T) {
	data, err := os.ReadFile("../extract/testdata/heading-5664.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		HTML string `json:"html"`
	}
	if err = json.Unmarshal(data, &f); err != nil {
		t.Fatal(err)
	}
	paragraphs, err := textparse.ParagraphsFromHTML([]byte(f.HTML))
	if err != nil {
		t.Fatal(err)
	}
	provisions := make(map[string]string)
	for _, p := range textparse.Parse(paragraphs).Provisions {
		provisions[p.Anchor] = p.Text
	}
	correction := source.Correction{Key: "vn:vbpl:2145", Number: "1/2024/NĐ-CP", Hold: true, Reason: "reviewed evidence", Reviewed: "2026-11-13", Evidence: source.CorrectionEvidence{Key: "vn:vbpl:5664", Anchor: "dieu-2/khoan-1", Quote: "Quyết định này có hiệu lực thi hành"}}
	docs := map[string]correctionDocument{
		correction.Key:          {number: correction.Number, provisions: map[string]string{correction.Evidence.Anchor: correction.Evidence.Quote}},
		correction.Evidence.Key: {number: "01/2000/QĐ-NHNN", provisions: provisions},
	}
	if _, err = validateCorrections([]source.Correction{correction}, docs); err != nil {
		t.Fatal(err)
	}
	// A missed preflight must still fail build, even if the quote exists on the target.
	provisions[correction.Evidence.Anchor] = strings.ReplaceAll(provisions[correction.Evidence.Anchor], "hiệu lực thi hành", "hiệu lực hành")
	if _, err = validateCorrections([]source.Correction{correction}, docs); err == nil || !strings.Contains(err.Error(), "evidence not found") {
		t.Fatalf("missing evidence error=%v", err)
	}
}
