package ocrcheck

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestCheck_AnswerDiagnosticUsage(t *testing.T) {
	cases := []struct {
		name, body, reason string
		want               map[string]any
	}{
		{"zero", `{"same_document":false,"pages":0,"items":[{},{}]}`, "schema:pages", map[string]any{"answer_pages": float64(0), "answer_same_document": false, "answer_items": float64(2), "answer_item_page_invalid": float64(2)}},
		{"fraction", `{"same_document":true,"pages":12.5,"items":[]}`, "schema:pages", map[string]any{"answer_pages_kind": "number", "answer_same_document": true, "answer_items": float64(0), "answer_item_page_invalid": float64(0)}},
		{"string", `{"same_document":true,"pages":"3","items":[]}`, "schema:pages", map[string]any{"answer_pages_kind": "string", "answer_same_document": true, "answer_items": float64(0), "answer_item_page_invalid": float64(0)}},
		{"missing", `{"same_document":false,"items":[]}`, "schema:pages:missing", map[string]any{"answer_same_document": false, "answer_items": float64(0), "answer_item_page_invalid": float64(0)}},
		{"above", `{"same_document":true,"pages":10,"items":[{"kind":"amount","page":11,"ocr":"x","pdf":"y","note":"z"}]}`, "schema:items[0].page", map[string]any{"answer_pages": float64(10), "answer_same_document": true, "answer_items": float64(1), "answer_item_page_max": float64(11), "answer_item_page_invalid": float64(1)}},
		{"zero page", `{"same_document":true,"pages":10,"items":[{"kind":"amount","page":0,"ocr":"x","pdf":"y","note":"z"}]}`, "schema:items[0].page", map[string]any{"answer_pages": float64(10), "answer_same_document": true, "answer_items": float64(1), "answer_item_page_max": float64(0), "answer_item_page_invalid": float64(1)}},
		{"string page", `{"same_document":true,"pages":10,"items":[{"kind":"amount","page":"1","ocr":"x","pdf":"y","note":"z"}]}`, "schema:items[0].page", map[string]any{"answer_pages": float64(10), "answer_same_document": true, "answer_items": float64(1), "answer_item_page_invalid": float64(1)}},
		{"zero pages", `{"same_document":true,"pages":0,"items":[{"page":1}]}`, "schema:pages", map[string]any{"answer_pages": float64(0), "answer_same_document": true, "answer_items": float64(1), "answer_item_page_max": float64(1), "answer_item_page_invalid": float64(1)}},
		{"negative pages", `{"same_document":true,"pages":-1,"items":[{"page":1}]}`, "schema:pages", map[string]any{"answer_pages": float64(-1), "answer_same_document": true, "answer_items": float64(1), "answer_item_page_max": float64(1), "answer_item_page_invalid": float64(1)}},
		{"nul reading", `{"same_document":true,"pages":10,"items":[{"kind":"amount","page":11,"ocr":"private\u0000reading","pdf":"y","note":"z"}]}`, "schema:nul", map[string]any{"answer_pages": float64(10), "answer_same_document": true, "answer_items": float64(1), "answer_item_page_max": float64(11), "answer_item_page_invalid": float64(1)}},
		{"valid", `{"same_document":true,"pages":10,"items":[]}`, "", map[string]any{}},
		{"null", `{"same_document":null,"pages":null,"items":null}`, "schema:same_document:null", map[string]any{"answer_pages_kind": "null"}},
		{"array", `{"same_document":true,"pages":[],"items":{}}`, "schema:pages", map[string]any{"answer_pages_kind": "array", "answer_same_document": true}},
		{"object", `{"same_document":true,"pages":{},"items":[]}`, "schema:pages", map[string]any{"answer_pages_kind": "object", "answer_same_document": true, "answer_items": float64(0), "answer_item_page_invalid": float64(0)}},
		{"bool", `{"same_document":true,"pages":false,"items":[]}`, "schema:pages", map[string]any{"answer_pages_kind": "bool", "answer_same_document": true, "answer_items": float64(0), "answer_item_page_invalid": float64(0)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, repo, client, docs := fixture(t)
			client.Response.Text = tc.body
			if err := run(t, r, docs, options()); err != nil {
				t.Fatal(err)
			}
			attempt := repo.attempts[0]
			if attempt.Reason != tc.reason {
				t.Fatalf("reason=%q want=%q", attempt.Reason, tc.reason)
			}
			data, err := json.Marshal(attempt.Usage)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(data), "private") {
				t.Fatal("usage contains answer text")
			}
			var usage map[string]any
			if err := json.Unmarshal(data, &usage); err != nil {
				t.Fatal(err)
			}
			got := map[string]any{}
			for _, key := range []string{"answer_pages", "answer_pages_kind", "answer_same_document", "answer_items", "answer_item_page_max", "answer_item_page_invalid"} {
				if value, ok := usage[key]; ok {
					got[key] = value
				}
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("diagnostic=%v want=%v", got, tc.want)
			}
		})
	}
}

func TestCheck_InvalidItemDiagnostics(t *testing.T) {
	cases := []struct {
		name, body, want string
	}{
		{"zero and unicode", `{"same_document":true,"pages":2,"items":[{"kind":"missing_text","page":0,"ocr":"","pdf":"bí mật","note":"ghi chú"}]}`, `[{"kind":"missing_text","ocr_chars":0,"pdf_chars":6,"note_chars":7}]`},
		{"unknown and nonstrings", `{"same_document":true,"pages":2,"items":[{"kind":"private-kind","page":"1","ocr":null,"pdf":5}]}`, `[{"kind":"other","ocr_chars":null,"pdf_chars":null,"note_chars":null}]`},
		{"missing and above", `{"same_document":true,"pages":2,"items":[{"page":1},{"kind":"date","ocr":"secret"},{"kind":"amount","page":3,"pdf":"hidden"}]}`, `[{"kind":"date","ocr_chars":6,"pdf_chars":null,"note_chars":null},{"kind":"amount","ocr_chars":null,"pdf_chars":6,"note_chars":null}]`},
		{"limit", `{"same_document":true,"pages":2,"items":[` + strings.Repeat(`{"page":0},`, 10) + `{"page":0}]}`, `[` + strings.Repeat(`{"kind":"other","ocr_chars":null,"pdf_chars":null,"note_chars":null},`, 9) + `{"kind":"other","ocr_chars":null,"pdf_chars":null,"note_chars":null}]`},
		{"valid", `{"same_document":true,"pages":10,"items":[]}`, `null`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, repo, client, docs := fixture(t)
			client.Response.Text = tc.body
			if err := run(t, r, docs, options()); err != nil {
				t.Fatal(err)
			}
			data, err := json.Marshal(repo.attempts[0].Usage)
			if err != nil {
				t.Fatal(err)
			}
			for _, secret := range []string{"bí mật", "ghi chú", "private-kind", "secret", "hidden"} {
				if strings.Contains(string(data), secret) {
					t.Fatalf("usage contains answer content: %q", secret)
				}
			}
			var usage map[string]json.RawMessage
			if err := json.Unmarshal(data, &usage); err != nil {
				t.Fatal(err)
			}
			got := usage["answer_invalid_items"]
			if len(got) == 0 {
				got = json.RawMessage("null")
			}
			if string(got) != tc.want {
				t.Fatalf("invalid items=%s want=%s", got, tc.want)
			}
		})
	}
}
