package extract

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/stage/fetch"
)

func TestCheck_StrictResultCannotBecomeIndexedClean(t *testing.T) {
	source := ocrFixture(t, true, 0)
	for hash, entry := range source.entries {
		entry.Check.Result = []byte(`{"same_document":false,"pages":1,"items":[],"verdict":"clean","Verdict":"rejected"}`)
		source.entries[hash] = entry
	}
	selected, err := selectOCRFiles(context.Background(), t.TempDir(), "1", []fetch.DocumentFile{{Name: "a.pdf", SHA256: sourceHash(source)}}, source, []string{"vision-ocr/builtin-latest"}, "checker")
	if err == nil || selected.source == "ocr" {
		t.Fatal("ambiguous result became indexable")
	}
}
func sourceHash(f fakeOCR) string {
	for h := range f.entries {
		return h
	}
	return ""
}
func TestCheck_SelectionReusesExtract(t *testing.T) {
	source := ocrFixture(t, false, 0)
	dir := headingBundle(t, `{"success":true,"data":[]}`, false)
	if err := os.WriteFile(filepath.Join(dir, "files.json"), []byte(`[{"name":"a.pdf","sha256":"`+sourceHash(source)+`","size":20}]`), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := SelectCheckSource(context.Background(), dir, "vn:vbpl:1", source, []string{"vision-ocr/builtin-latest"}, "checker")
	if err != nil || got == nil || got.Entry.PdfSha256 != sourceHash(source) {
		t.Fatalf("candidate=%v err=%v", got, err)
	}
}

func TestCheck_LabelFlagDoesNotChangeSelection(t *testing.T) {
	source := ocrFixture(t, true, 1)
	hash := sourceHash(source)
	entry := source.entries[hash]
	entry.Check.Result = []byte(`{"same_document":true,"pages":1,"pdf_pages":1,"items":[{"kind":"article_label","page":1,"ocr":"1","pdf":"2","note":"x"}],"verdict":"flagged"}`)
	source.entries[hash] = entry
	dir := headingBundle(t, `{"success":true,"data":[]}`, false)
	if err := os.WriteFile(filepath.Join(dir, "files.json"), []byte(`[{"name":"a.pdf","sha256":"`+hash+`","size":20}]`), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := SelectCheckSource(context.Background(), dir, "vn:vbpl:1", source, []string{"vision-ocr/builtin-latest"}, "checker")
	if err != nil || got == nil || got.Entry.PdfSha256 != hash {
		t.Fatalf("candidate=%v err=%v", got, err)
	}
}
