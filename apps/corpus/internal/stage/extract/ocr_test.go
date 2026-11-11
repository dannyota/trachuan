package extract

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/ocrstore"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/fetch"
	"danny.vn/trachuan/apps/corpus/internal/store"
)

type fakeOCR struct {
	entries map[string]ocrstore.Entry
	data    []byte
}

func (f fakeOCR) Lookup(_ context.Context, hashes, engines []string, checker string) (map[string]ocrstore.Entry, error) {
	out := map[string]ocrstore.Entry{}
	for _, hash := range hashes {
		if e, ok := f.entries[hash]; ok {
			for _, engine := range engines {
				if e.Text.EngineID == engine {
					if e.Check != nil && e.Check.CheckerID != checker {
						e.Check = nil
					}
					out[hash] = e
					break
				}
			}
		}
	}
	return out, nil
}
func (f fakeOCR) ReadText(store.OcrText) ([]byte, error) { return f.data, nil }

func ocrFixture(t *testing.T, checked bool, flagged int32) fakeOCR {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "textparse", "testdata", "ocr_decree_142_2026.txt"))
	if err != nil {
		t.Fatal(err)
	}
	entry := ocrstore.Entry{Text: store.OcrText{PdfSha256: strings.Repeat("a", 64), EngineID: "vision-ocr/builtin-latest", TextSha256: strings.Repeat("b", 64)}}
	if checked {
		entry.Check = &store.OcrCheck{CheckerID: "checker", ResultSha256: strings.Repeat("c", 64), Flagged: flagged}
	}
	return fakeOCR{entries: map[string]ocrstore.Entry{entry.Text.PdfSha256: entry}, data: data}
}
func TestExtract_OCRNeedsCompletedCheck(t *testing.T) {
	files := []fetch.DocumentFile{{Name: "source.pdf", SHA256: strings.Repeat("a", 64)}}
	s, err := selectOCRFiles(context.Background(), t.TempDir(), "1", files, ocrFixture(t, false, 0), []string{"vision-ocr/builtin-latest"}, "checker")
	if err != nil || s.source != "none" || s.gap != "needs_ocr" || selectionInputHash("fetch", s) != InputHash("fetch", "none", "needs_ocr") {
		t.Fatalf("selection=%+v err=%v", s, err)
	}
	s, err = selectOCRFiles(context.Background(), t.TempDir(), "1", files, ocrFixture(t, true, 0), []string{"vision-ocr/builtin-latest"}, "checker")
	if err != nil || s.source != "ocr" {
		t.Fatalf("selection=%+v err=%v", s, err)
	}
	rows, gaps, rec := parseOCR(s, ocrFixture(t, true, 0).data)
	if len(rows) == 0 || rec.TextSource != "ocr" || len(rec.Provisions) == 0 || rec.Provisions[0].StartLine <= 0 || rec.Provisions[0].EndLine < rec.Provisions[0].StartLine || hasGap(gaps, "needs_ocr", "") {
		t.Fatalf("OCR output: rows=%d provisions=%d source=%q gaps=%v", len(rows), len(rec.Provisions), rec.TextSource, gaps)
	}
}
func TestExtract_OCRFlaggedGap(t *testing.T) {
	src := ocrFixture(t, true, 2)
	s, err := selectOCRFiles(context.Background(), t.TempDir(), "1", []fetch.DocumentFile{{Name: "a.pdf", SHA256: strings.Repeat("a", 64)}}, src, []string{"vision-ocr/builtin-latest"}, "checker")
	if err != nil {
		t.Fatal(err)
	}
	rows, gaps, _ := parseOCR(s, src.data)
	if len(rows) == 0 || !hasGap(gaps, "ocr_flagged", "") {
		t.Fatalf("rows=%d gaps=%v", len(rows), gaps)
	}
}
func TestExtract_SinglePDFSelection(t *testing.T) {
	src := ocrFixture(t, true, 0)
	files := []fetch.DocumentFile{{Name: "a.pdf", SHA256: strings.Repeat("a", 64)}, {Name: "b.PDF", SHA256: strings.Repeat("a", 64)}}
	s, err := selectOCRFiles(context.Background(), t.TempDir(), "1", files, src, []string{"vision-ocr/builtin-latest"}, "checker")
	if err != nil || s.source != "ocr" || s.name != "a.pdf" {
		t.Fatalf("selection=%+v err=%v", s, err)
	}
}
func TestExtract_AmbiguousPDFsGap(t *testing.T) {
	src := ocrFixture(t, true, 0)
	other := src.entries[strings.Repeat("a", 64)]
	other.Text.PdfSha256 = strings.Repeat("d", 64)
	src.entries[other.Text.PdfSha256] = other
	files := []fetch.DocumentFile{{Name: "a.pdf", SHA256: strings.Repeat("a", 64)}, {Name: "b.pdf", SHA256: strings.Repeat("d", 64)}}
	s, err := selectOCRFiles(context.Background(), t.TempDir(), "1", files, src, []string{"vision-ocr/builtin-latest"}, "checker")
	if err != nil || s.gap != "text_source_ambiguous" {
		t.Fatalf("selection=%+v err=%v", s, err)
	}
	delete(src.entries, other.Text.PdfSha256)
	s, err = selectOCRFiles(context.Background(), t.TempDir(), "1", files, src, []string{"vision-ocr/builtin-latest"}, "checker")
	if err != nil || s.source != "ocr" || s.name != "a.pdf" {
		t.Fatalf("single stored PDF selection=%+v err=%v", s, err)
	}
	src.entries[strings.Repeat("a", 64)] = ocrFixture(t, false, 0).entries[strings.Repeat("a", 64)]
	s, err = selectOCRFiles(context.Background(), t.TempDir(), "1", files, src, []string{"vision-ocr/builtin-latest"}, "checker")
	if err != nil || s.gap != "needs_ocr" {
		t.Fatalf("unchecked stored PDF selection=%+v err=%v", s, err)
	}
}
func TestExtract_OCRInputHashFreezesEntryAndCheck(t *testing.T) {
	text, check := strings.Repeat("b", 64), strings.Repeat("c", 64)
	h := fetch.ExtractInputHash("fetch", "ocr", text, check)
	if h == fetch.ExtractInputHash("fetch", "ocr", strings.Repeat("d", 64), check) || h == fetch.ExtractInputHash("fetch", "ocr", text, strings.Repeat("e", 64)) {
		t.Fatal("hash did not freeze OCR text and check")
	}
	dir := t.TempDir()
	body := `{"text_source":"ocr","text_sha256":"` + text + `","result_sha256":"` + check + `"}`
	if err := os.WriteFile(filepath.Join(dir, "text.json"), []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	if err := fetch.CheckExtractLineage(dir, "fetch", h); err != nil {
		t.Fatal(err)
	}
	if err := fetch.CheckExtractLineage(dir, "fetch", fetch.ExtractInputHash("fetch", "ocr", text, strings.Repeat("e", 64))); err == nil {
		t.Fatal("accepted changed check")
	}
	if err := os.WriteFile(filepath.Join(dir, "text.json"), []byte(`{"text_source":"ocr","text_sha256":"invalid","result_sha256":"`+check+`"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := fetch.CheckExtractLineage(dir, "fetch", h); err == nil {
		t.Fatal("accepted invalid OCR hash")
	}
}

func TestExtract_HeadingOnlyUsesCheckedOCR(t *testing.T) {
	dir := headingBundle(t, bareTree, false)
	hash := strings.Repeat("a", 64)
	if err := os.WriteFile(filepath.Join(dir, "files.json"), []byte(`[{"name":"scan.pdf","sha256":"`+hash+`","size":1}]`), 0600); err != nil {
		t.Fatal(err)
	}
	src := ocrFixture(t, true, 0)
	selected, nodes, err := chooseWithOCR(context.Background(), dir, "vn:vbpl:1", src, []string{"vision-ocr/builtin-latest"}, "checker")
	if err != nil || selected.source != "ocr" || len(nodes) != 0 {
		t.Fatalf("selection=%+v nodes=%d err=%v", selected, len(nodes), err)
	}
	src.data = []byte("short")
	selected, nodes, err = chooseWithOCR(context.Background(), dir, "vn:vbpl:1", src, []string{"vision-ocr/builtin-latest"}, "checker")
	if err != nil || selected.source != "source_tree" || selected.gap != "tree_headings_only" || len(nodes) == 0 {
		t.Fatalf("selection=%+v nodes=%d err=%v", selected, len(nodes), err)
	}
}

func TestExtract_OCRRecipeIncludesSettings(t *testing.T) {
	base := New().Recipe(stage.Snapshot{})
	otherChecker := New().WithOCR(nil, nil, "checker").Recipe(stage.Snapshot{})
	otherEngine := New().WithOCR(nil, []string{"another"}, "").Recipe(stage.Snapshot{})
	if base == otherChecker || base == otherEngine || otherChecker == otherEngine {
		t.Fatal("recipe omitted OCR settings")
	}
}

func TestExtract_OCRLineRangesIncludeBlankLines(t *testing.T) {
	src := ocrFixture(t, true, 0)
	selected := selection{source: "ocr", name: "a.pdf", hash: strings.Repeat("a", 64)}
	entry := src.entries[selected.hash]
	selected.entry = &entry
	data := []byte("Điều 1. Quy định chung\n\n" + strings.Repeat("nội dung ", 45) + "\n\nĐiều 2. Thi hành\n" + strings.Repeat("quy định ", 45))
	_, _, record := parseOCR(selected, data)
	var found bool
	for _, provision := range record.Provisions {
		if provision.Anchor == "dieu-1" {
			found = true
			if provision.StartLine != 1 || provision.EndLine != 3 {
				t.Fatalf("line range %d-%d", provision.StartLine, provision.EndLine)
			}
		}
	}
	if !found {
		t.Fatal("missing first article")
	}
}

func TestExtract_UncheckedEntryChangesGapArtifact(t *testing.T) {
	ctx := context.Background()
	files := []fetch.DocumentFile{{Name: "a.pdf", SHA256: strings.Repeat("a", 64)}, {Name: "b.pdf", SHA256: strings.Repeat("b", 64)}}
	src := ocrFixture(t, false, 0)
	entry := src.entries[strings.Repeat("a", 64)]
	delete(src.entries, strings.Repeat("a", 64))
	artifacts := artifact.Store{Root: t.TempDir()}
	publish := func() (selection, artifact.Result) {
		t.Helper()
		selected, err := selectOCRFiles(ctx, t.TempDir(), "1", files, src, []string{"vision-ocr/builtin-latest"}, "checker")
		if err != nil {
			t.Fatal(err)
		}
		result, err := artifacts.Publish(ctx, "extract", selectionInputHash(strings.Repeat("c", 64), selected), strings.Repeat("d", 64), func(dir string) error {
			return os.WriteFile(filepath.Join(dir, "report.json"), []byte(selected.gap), 0600)
		})
		if err != nil {
			t.Fatal(err)
		}
		return selected, result
	}
	before, first := publish()
	src.entries[strings.Repeat("a", 64)] = entry
	after, second := publish()
	if before.gap != "text_source_ambiguous" || after.gap != "needs_ocr" {
		t.Fatalf("gaps %q, %q", before.gap, after.gap)
	}
	if first.InputHash == second.InputHash || first.Path == second.Path || second.Adopted {
		t.Fatal("reused artifact with a different gap")
	}
	data, err := os.ReadFile(filepath.Join(artifacts.Abs(second.Path), "report.json"))
	if err != nil || string(data) != "needs_ocr" {
		t.Fatalf("gap %q: %v", data, err)
	}
}
