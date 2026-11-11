package ocrcheck

import (
	"encoding/json"
	"testing"
)

func TestCheck_ItemReadings(t *testing.T) {
	for _, kind := range []string{"document_number", "date", "article_label", "amount", "extra_text", "missing_text"} {
		for _, ocr := range []string{"", " \n\t", "legal"} {
			for _, pdf := range []string{"", " \n\t", "different"} {
				t.Run(kind+"/"+ocr+"/"+pdf, func(t *testing.T) {
					body, _ := json.Marshal(map[string]any{"same_document": true, "pages": 10, "items": []map[string]any{{"kind": kind, "page": 1, "ocr": ocr, "pdf": pdf, "note": "reading"}}})
					result, reason := decodeAnswer(body, 10, []byte("legal"))
					presentOCR := ocr == "legal"
					presentPDF := pdf == "different"
					valid := presentOCR && presentPDF
					if kind == "extra_text" {
						valid = presentOCR && !presentPDF
					}
					if kind == "missing_text" {
						valid = !presentOCR && presentPDF
					}
					if !valid {
						if reason != "item_readings" {
							t.Fatalf("reason=%q want item_readings", reason)
						}
						return
					}
					if reason != "" || result.Verdict != "flagged" || len(result.Items) != 1 || result.DroppedEqual != 0 {
						t.Fatalf("result=%+v reason=%s", result, reason)
					}
				})
			}
		}
	}
}
func TestCheck_MissingTextCannotCleanVerdict(t *testing.T) {
	for _, same := range []bool{true, false} {
		for _, count := range []int{1, 2} {
			items := make([]map[string]any, count)
			for i := range items {
				items[i] = map[string]any{"kind": "missing_text", "page": i + 1, "ocr": " \n", "pdf": "missing legal text", "note": "claim for review"}
			}
			body, _ := json.Marshal(map[string]any{"same_document": same, "pages": 10, "items": items})
			result, reason := decodeAnswer(body, 10, nil)
			want := "flagged"
			if !same || count == 2 {
				want = "rejected"
			}
			if reason != "" || result.Verdict != want || len(result.Items) != count || result.DroppedEqual != 0 {
				t.Fatalf("result=%+v reason=%s", result, reason)
			}
		}
	}
}
func TestCheck_NumberSpacingStaysDifferent(t *testing.T) {
	result, reason := decodeAnswer([]byte(`{"same_document":true,"pages":10,"items":[{"kind":"amount","page":1,"ocr":"1 000","pdf":"1000","note":"number"}]}`), 10, []byte("1 000"))
	if reason != "" || result.Verdict != "flagged" || len(result.Items) != 1 || result.DroppedEqual != 0 {
		t.Fatalf("result=%+v reason=%s", result, reason)
	}
}
