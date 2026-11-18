package app

import (
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"
)

var updateGolden = flag.Bool("update-intake-golden", false, "rewrite the batch fixture the corpus intake reads")

func goldenRows() []exportReport {
	at := time.Date(2099, 11, 18, 0, 0, 0, 0, time.UTC)
	row := func(id int64, key, anchor, kind, target string, suggestion map[string]string) exportReport {
		return exportReport{ReportID: id, CreatedAt: at, DocumentKey: key, Anchor: anchor, DocumentNumber: "1/2099/ND-CP", Release: "20991118_1", Kind: kind, Target: target, Suggestion: suggestion}
	}
	valid := map[string]string{"status_class": "expired", "eff_to": "2099-01-01"}
	return []exportReport{
		row(1, "vn:vbpl:1", "", "wrong_validity", "", valid),
		row(2, "vn:vbpl:1", "", "wrong_validity", "", valid),
		row(3, "vn:vbpl:1", "dieu-5", "wrong_validity", "", map[string]string{}),
		row(4, "vn:vbpl:2", "dieu-5~2", "wrong_text", "", map[string]string{}),
		row(5, "vn:vbpl:2", "dieu-5/diem-đ", "wrong_text", "", map[string]string{}),
		row(6, "vn:chinhphu:hhtquyetdinh_65", "", "wrong_text", "", map[string]string{}),
		row(7, "vn:vbpl:3", "dieu-2", "broken_citation", "5/2099/ND-CP", map[string]string{"document_number": "5/2099/ND-CP"}),
		row(8, "", "", "missing_document", "9/2099/ND-CP", map[string]string{"document_number": "9/2099/ND-CP"}),
		row(9, "", "", "missing_document", "9/2099/ND-CP", map[string]string{"document_number": "9/2099/ND-CP"}),
		row(10, "vn:vbpl:3", "dieu-2", "missing_document", "12/2099/QÐ-UBND & 3", map[string]string{"document_number": "12/2099/QÐ-UBND & 3"}),
	}
}

func TestWriteReportExport_MatchesTheCorpusIntakeFixture(t *testing.T) {
	fixture := filepath.Join("..", "..", "..", "corpus", "internal", "intake", "testdata", "batch", "b2099")
	dir := filepath.Join(t.TempDir(), "b2099")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := writeReportExport(dir, "b2099", goldenRows()); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if *updateGolden {
		if err = os.MkdirAll(fixture, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, e := range entries {
		got, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if *updateGolden {
			if err = os.WriteFile(filepath.Join(fixture, e.Name()), got, 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		want, err := os.ReadFile(filepath.Join(fixture, e.Name()))
		if err != nil || string(want) != string(got) {
			t.Errorf("%s differs from the pinned fixture (run with -update-intake-golden to regenerate it)", e.Name())
		}
	}
	if !*updateGolden {
		pinned, err := os.ReadDir(fixture)
		if err != nil || len(pinned) != len(entries) {
			t.Fatalf("fixture holds %d files, export writes %d: %v", len(pinned), len(entries), err)
		}
	}
}
