package ocrstore

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestCheck_ValidatesVerdictAndCoverage(t *testing.T) {
	tests := []struct {
		name, body string
		flagged    int32
		pages      pgtype.Int4
		ok         bool
	}{
		{"clean", `{"same_document":true,"pages":10,"items":[],"verdict":"clean"}`, 0, pgtype.Int4{Int32: 10, Valid: true}, true},
		{"flagged", `{"same_document":true,"pages":10,"items":[{"kind":"amount","page":2,"ocr":"1","pdf":"2","note":"difference"}],"verdict":"flagged"}`, 1, pgtype.Int4{}, true},
		{"rejected", `{"same_document":false,"pages":10,"items":[],"verdict":"rejected"}`, 0, pgtype.Int4{}, true},
		{"missing coverage", `{"same_document":true,"pages":9,"items":[],"verdict":"clean"}`, 0, pgtype.Int4{Int32: 10, Valid: true}, false},
		{"missing page", `{"same_document":true,"pages":0,"items":[],"verdict":"clean"}`, 0, pgtype.Int4{}, false},
		{"unknown kind", `{"same_document":true,"pages":1,"items":[{"kind":"unknown","page":1,"ocr":"1","pdf":"2","note":"x"}],"verdict":"flagged"}`, 1, pgtype.Int4{}, false},
		{"wrong count", `{"same_document":true,"pages":1,"items":[],"verdict":"clean"}`, 1, pgtype.Int4{}, false},
		{"wrong verdict", `{"same_document":false,"pages":1,"items":[],"verdict":"clean"}`, 0, pgtype.Int4{}, false},
		{"missing fields", `{"same_document":true,"pages":1,"items":[{"kind":"amount","page":1}],"verdict":"flagged"}`, 1, pgtype.Int4{}, false},
		{"missing text threshold", `{"same_document":true,"pages":10,"items":[{"kind":"missing_text","page":1,"ocr":"","pdf":"a","note":"x"},{"kind":"missing_text","page":2,"ocr":"","pdf":"b","note":"x"}],"verdict":"rejected"}`, 2, pgtype.Int4{}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := validateCheck([]byte(tc.body), tc.flagged, tc.pages)
			if (err == nil) != tc.ok {
				t.Fatalf("valid=%v error=%v", tc.ok, err)
			}
		})
	}
}
