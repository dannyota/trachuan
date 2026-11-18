package extract

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/ocrstore"
	"danny.vn/trachuan/apps/corpus/internal/stage/fetch"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestWaiver_MatchesExactItems(t *testing.T) {
	w := Waiver{Key: "vn:test:1", PDFSHA256: strings.Repeat("a", 64), CheckSHA256: strings.Repeat("b", 64), Items: []WaiverItem{{Page: 1, Kind: "document_number", OCR: "a", PDF: "b", Reading: "form code"}}}
	result := ocrstore.CheckResult{Verdict: "flagged", Items: []ocrstore.CheckItem{{Page: 1, Kind: "document_number", OCR: "a", PDF: "b"}}}
	entry := ocrstore.Entry{Check: &store.OcrCheck{ResultSha256: w.CheckSHA256}}
	ok, stale, err := matchWaiver(&w, w.PDFSHA256, entry, result)
	if err != nil || !ok || stale {
		t.Fatalf("match=%v stale=%v err=%v", ok, stale, err)
	}
	result.Items = append(result.Items, ocrstore.CheckItem{Page: 2, Kind: "article_label"})
	if _, _, err = matchWaiver(&w, w.PDFSHA256, entry, result); err == nil {
		t.Fatal("missing item accepted")
	}
	_, stale, err = matchWaiver(&w, strings.Repeat("c", 64), entry, result)
	if err != nil || !stale {
		t.Fatalf("stale=%v err=%v", stale, err)
	}
	changed := ocrstore.CheckResult{Verdict: "flagged", Items: []ocrstore.CheckItem{{Page: 1, Kind: "document_number", OCR: "a", PDF: "c"}}}
	if _, _, err = matchWaiver(&w, w.PDFSHA256, entry, changed); err == nil {
		t.Fatal("changed reading accepted")
	}
	pair := Waiver{Key: w.Key, PDFSHA256: w.PDFSHA256, CheckSHA256: w.CheckSHA256, Items: []WaiverItem{{Page: 1, Kind: "document_number", OCR: "a", PDF: "b", Reading: "form code"}, {Page: 1, Kind: "document_number", OCR: "a", PDF: "b", Reading: "form code"}}}
	two := ocrstore.CheckResult{Verdict: "flagged", Items: []ocrstore.CheckItem{{Page: 1, Kind: "document_number", OCR: "a", PDF: "b"}, {Page: 1, Kind: "document_number", OCR: "x", PDF: "y"}}}
	if _, _, err = matchWaiver(&pair, pair.PDFSHA256, entry, two); err == nil {
		t.Fatal("duplicate substitution accepted")
	}
}

func TestWaiver_ParseRejectsUnknownFields(t *testing.T) {
	if _, _, err := parseWaivers([]byte("- unknown: 1\n")); err == nil {
		t.Fatal("unknown field accepted")
	}
}

func TestWaiver_SelectsOnlyMatchingCheck(t *testing.T) {
	src := ocrFixture(t, true, 1)
	hash := strings.Repeat("a", 64)
	entry := src.entries[hash]
	result := ocrstore.CheckResult{SameDocument: true, Pages: 10, PDFPages: 10, Verdict: "flagged", Items: []ocrstore.CheckItem{{Kind: "document_number", Page: 1, OCR: "1", PDF: "2", Note: "form"}}}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	entry.Check.Result = data
	src.entries[hash] = entry
	w := &Waiver{Key: "vn:test:1", PDFSHA256: hash, CheckSHA256: entry.Check.ResultSha256, Items: []WaiverItem{{Page: 1, Kind: "document_number", OCR: "1", PDF: "2"}}}
	files := []fetch.DocumentFile{{Name: "scan.pdf", SHA256: hash}}
	choose := func() (selection, error) {
		return selectOCRFilesMode(context.Background(), t.TempDir(), "1", files, src, []string{"vision-ocr/builtin-latest"}, "checker", false, w)
	}
	got, err := choose()
	if err != nil || got.source != "ocr" {
		t.Fatalf("selection=%+v err=%v", got, err)
	}
	_, gaps, _ := parseOCR(got, src.data)
	if !hasGap(gaps, "ocr_flagged", "") {
		t.Fatalf("gaps=%v", gaps)
	}
	w.CheckSHA256 = strings.Repeat("d", 64)
	got, err = choose()
	if err != nil || got.gap != "ocr_label_flagged" {
		t.Fatalf("stale selection=%+v err=%v", got, err)
	}
	w.CheckSHA256 = entry.Check.ResultSha256
	w.Items[0].Page = 2
	if _, err = choose(); err == nil {
		t.Fatal("accepted mismatched items")
	}
	result.SameDocument = false
	result.Verdict = "rejected"
	data, err = json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	entry.Check.Result = data
	src.entries[hash] = entry
	if _, err = choose(); err == nil {
		t.Fatal("accepted mismatched items on rejected check")
	}
}

func TestWaiver_ParseAndCanonicalHash(t *testing.T) {
	path := filepath.Join("..", "..", "..", "..", "..", "sources", "vn-ocr-waivers.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	entries, hash, err := parseWaivers(data)
	if err != nil || len(entries) == 0 || len(hash) != 64 {
		t.Fatalf("entries=%d hash=%q err=%v", len(entries), hash, err)
	}
	for _, suffix := range []string{"\n---\n[]\n", "\n  unknown: value\n"} {
		if _, _, err = parseWaivers(append(append([]byte{}, data...), []byte(suffix)...)); err == nil {
			t.Fatalf("accepted %q", suffix)
		}
	}
	changed := strings.Replace(string(data), "2026-11-15", "2026-11-16", 1)
	_, next, err := parseWaivers([]byte(changed))
	if err != nil || next == hash {
		t.Fatalf("hash did not change: %v", err)
	}
}

func TestWaiver_UnknownSelectionStopsPlanning(t *testing.T) {
	w := Waivers{entries: []Waiver{{Key: "vn:test:1"}}}
	if err := w.validateSelections(map[string]bool{}); err == nil {
		t.Fatal("accepted missing selection")
	}
	if err := w.validateSelections(map[string]bool{"vn:test:1": true}); err != nil {
		t.Fatal(err)
	}
}

func TestWaiver_FrozenReplayRecordsWaiver(t *testing.T) {
	pdf, text, check, engine, checker := strings.Repeat("a", 64), strings.Repeat("b", 64), strings.Repeat("c", 64), "engine", "checker"
	id, textID, checkID := int64(1), int64(2), int64(3)
	q := &frozenDB{rows: map[store.GetRunOCRSelectionParams]store.GetRunOCRSelectionRow{
		{RunID: id, ItemKey: "vn:test:1"}: {PdfSha256: &pdf, OcrTextID: &textID, OcrCheckID: &checkID, EngineID: &engine, TextSha256: &text, CheckerID: &checker, ResultSha256: &check, Flagged: pgtype.Int4{Int32: 1, Valid: true}},
	}}
	got, err := planFrozenSelection(context.Background(), q, id, "vn:test:1", func() (selection, error) { t.Fatal("consulted live store"); return selection{}, nil })
	if err != nil || got.source != "ocr" {
		t.Fatalf("selection=%+v err=%v", got, err)
	}
	w := &Waiver{Key: "vn:test:1", PDFSHA256: pdf, CheckSHA256: check}
	record := &textRecord{}
	gaps := annotateWaiver(w, got, record, nil)
	if len(gaps) != 0 || record.Waiver == nil || record.Waiver.Key != w.Key || record.Waiver.CheckSHA256 != check {
		t.Fatalf("record=%+v gaps=%v", record, gaps)
	}
	w.CheckSHA256 = strings.Repeat("d", 64)
	gaps = annotateWaiver(w, got, &textRecord{}, nil)
	if !hasGap(gaps, "ocr_waiver_stale", "waiver did not apply to the frozen OCR selection: vn:test:1") {
		t.Fatalf("stale waiver not reported: %v", gaps)
	}
}

func TestReadWaivers_MissingAndEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "waivers.yaml")
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ReadWaivers(path); err == nil || !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), cwd) {
		t.Fatalf("missing file error: %v", err)
	}
	var hash string
	for _, data := range []string{"[]\n", ""} {
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		w, err := ReadWaivers(path)
		if err != nil || len(w.entries) != 0 {
			t.Fatalf("%q: %v", data, err)
		}
		if hash != "" && hash != w.hash {
			t.Fatal("empty input hash differs from empty list")
		}
		hash = w.hash
	}
}
