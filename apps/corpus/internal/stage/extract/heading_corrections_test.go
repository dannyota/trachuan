package extract

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/source"
)

func TestHeadingCorrections_CrossDocumentVerbatimEvidence(t *testing.T) {
	f := recordedHeadingFixture(t)
	dir := transcriptionBundle(t, string(f.Tree), f.HTML)
	entries := []source.Correction{
		{Key: "vn:vbpl:2145", Evidence: source.CorrectionEvidence{Key: "vn:vbpl:1", Anchor: "dieu-2/khoan-1", Quote: "Quyết định này có hiệu lực thi hành"}},
		{Key: "vn:vbpl:2", Evidence: source.CorrectionEvidence{Key: "vn:vbpl:1", Anchor: "dieu-2/khoan-1", Quote: "hiệu lực  thi hành"}},
		{Key: "vn:vbpl:3", Evidence: source.CorrectionEvidence{Key: "vn:vbpl:1", Anchor: "dieu-3", Quote: "Quyết định"}},
		{Key: "vn:vbpl:4", Evidence: source.CorrectionEvidence{Anchor: "dieu-2", Quote: "Quyết định"}},
	}
	checks, err := CheckHeadingCorrections(context.Background(), entries, map[string]string{"vn:vbpl:1": dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(checks) != 3 || checks[0].Key != "vn:vbpl:2145" || checks[0].EvidenceKey != "vn:vbpl:1" || !checks[0].QuoteFound || checks[1].QuoteFound || checks[2].QuoteFound {
		t.Fatalf("checks=%+v", checks)
	}
	entries = entries[:1]
	entries[0].Key = "vn:vbpl:1"
	entries[0].Evidence.Key = ""
	checks, err = CheckHeadingCorrections(context.Background(), entries, map[string]string{"vn:vbpl:1": dir})
	if err != nil || len(checks) != 1 || !checks[0].QuoteFound {
		t.Fatalf("checks=%+v err=%v", checks, err)
	}
}

func TestHeadingCorrections_RecordedCrossDocumentCorrection(t *testing.T) {
	data, err := os.ReadFile("testdata/heading-10761.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Tree       json.RawMessage   `json:"tree"`
		HTML       string            `json:"html"`
		Correction source.Correction `json:"correction"`
	}
	if err = json.Unmarshal(data, &f); err != nil {
		t.Fatal(err)
	}
	if f.Correction.Key != "vn:vbpl:2145" || f.Correction.Evidence.Key != "vn:vbpl:10761" || f.Correction.Evidence.Anchor != "dieu-20" {
		t.Fatal("recorded correction identity changed")
	}
	dir := transcriptionBundle(t, string(f.Tree), f.HTML)
	files, err := os.ReadFile(filepath.Join(dir, "files.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "files.json"), []byte(strings.ReplaceAll(string(files), "1_content.html", "10761_content.html")), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "detail.json"), []byte(`{"data":{"id":"10761"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	checks, err := CheckHeadingCorrections(context.Background(), []source.Correction{f.Correction}, map[string]string{"vn:vbpl:10761": dir})
	if err != nil || len(checks) != 1 || !checks[0].QuoteFound || checks[0].EvidenceKey != "vn:vbpl:10761" {
		t.Fatalf("checks=%+v err=%v", checks, err)
	}
}
