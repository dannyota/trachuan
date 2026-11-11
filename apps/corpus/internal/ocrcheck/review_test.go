package ocrcheck

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/ocrstore"
	"danny.vn/trachuan/apps/corpus/internal/store"
)

func TestReview_FileListsItemsWithAnchors(t *testing.T) {
	text := []byte("Điều 1. Phạm vi điều chỉnh\nQuy định mức 100 đồng.\n\nĐiều 2. Thực hiện\nCơ quan thực hiện quy định này.\n" + strings.Repeat("Nội dung thực hiện quy định. ", 20))
	result := ocrstore.CheckResult{SameDocument: true, Pages: 10, PDFPages: 10, Verdict: "flagged", Items: []ocrstore.CheckItem{{Kind: "amount", Page: 1, OCR: "100 đồng", PDF: "200 đồng", Note: "amount"}}}
	doc, err := Review("vn:vbpl:1", "source.pdf", store.OcrText{ID: 1}, "checker", result, text)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "ocr-review.json")
	if err = WriteReview(path, []ReviewDocument{doc}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got []ReviewDocument
	if err = json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || len(got[0].Items) != 1 || !strings.Contains(strings.Join(got[0].Items[0].Anchors, ","), "dieu-1") {
		t.Fatalf("review=%s", data)
	}
}
