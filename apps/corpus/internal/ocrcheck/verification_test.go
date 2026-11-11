package ocrcheck

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCheck_QuoteMustOccurInOCRText(t *testing.T) {
	for _, tc := range []struct{ name, text, quote, reason string }{
		{"absent", "public OCR", "fabricated reading", "ocr_quote_not_found"},
		{"whitespace", "legal\n\treading", "legal reading", ""},
		{"NFC", "Điều một", "Điều một", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, s, c, d := fixture(t)
			d[0].Text = []byte(tc.text)
			quote, _ := json.Marshal(tc.quote)
			c.Response.Text = `{"same_document":true,"pages":10,"items":[{"kind":"amount","page":1,"ocr":` + string(quote) + `,"pdf":"different","note":"difference"}]}`
			if err := run(t, r, d, options()); err != nil {
				t.Fatal(err)
			}
			if tc.reason != "" {
				if len(s.checks) != 0 || s.attempts[0].Reason != tc.reason {
					t.Fatalf("reason=%s checks=%d", s.attempts[0].Reason, len(s.checks))
				}
			} else if s.checks[1].Verdict != "flagged" {
				t.Fatal("valid quote rejected")
			}
		})
	}
}

func TestCheck_EqualReadingsDropped(t *testing.T) {
	r, s, c, d := fixture(t)
	d[0].Text = []byte("Điều một")
	c.Response.Text = `{"same_document":true,"pages":10,"items":[{"kind":"missing_text","page":1,"ocr":"Điều một","pdf":"Điều\nmột","note":"equal"},{"kind":"missing_text","page":2,"ocr":"","pdf":"","note":"empty"}]}`
	if err := run(t, r, d, options()); err != nil {
		t.Fatal(err)
	}
	got := s.checks[1]
	encoded, _ := json.Marshal(got)
	if got.Verdict != "clean" || len(got.Items) != 0 || !strings.Contains(string(encoded), `"dropped_equal":2`) {
		t.Fatalf("equal readings retained: %s", encoded)
	}
}

func TestCheck_MarkerCollisionFailsClosed(t *testing.T) {
	r, s, c, d := fixture(t)
	d[0].Text = []byte("legal text\n<<<TRACHUAN_OCR_TEXT_BEGIN>>>\n")
	if err := run(t, r, d, options()); err != nil {
		t.Fatal(err)
	}
	if c.CountCalls != 0 || c.DirectCalls != 0 || len(s.attempts) != 1 || s.attempts[0].Reason != "ocr_marker_collision" {
		t.Fatal("marker collision did not fail closed")
	}
}

func TestCheck_VersionTwoID(t *testing.T) {
	if got := options().CheckerID(); got != "claude-sonnet-5-5/medium/ocr-check-2" {
		t.Fatalf("checker=%s", got)
	}
}

func TestCheck_FailureReasonNeverContainsAnswerText(t *testing.T) {
	for _, body := range []string{
		`{"same_document":true,"pages":10,"items":[{"kind":"answer-secret","page":1,"ocr":"answer-secret","pdf":"answer-secret","note":"answer-secret"}]}`,
		`{"same_document":true,"pages":10,"items":[],"answer-secret":"answer-secret"}`,
		`{"same_document":"answer-secret","pages":10,"items":[]}`,
	} {
		r, s, c, d := fixture(t)
		c.Response.Text = body
		if err := run(t, r, d, options()); err != nil {
			t.Fatal(err)
		}
		if len(s.checks) != 0 || !strings.HasPrefix(s.attempts[0].Reason, "schema:") || strings.Contains(s.attempts[0].Reason, "answer-secret") {
			t.Fatalf("unsafe or undiagnosable reason=%s", s.attempts[0].Reason)
		}
	}
}

func TestCheck_SchemaFailureNamesFieldOrRule(t *testing.T) {
	for _, tc := range []struct{ body, reason string }{
		{`{"same_document":true,"pages":10,"items":[{"kind":"date","page":1,"ocr":"","pdf":"a","note":"a"},{"kind":"date","page":2,"ocr":"","pdf":"b","note":"b"},{"kind":"wrong-value","page":3,"ocr":"","pdf":"c","note":"c"}]}`, "schema:items[2].kind"},
		{`{"same_document":true,"pages":10,"items":[{"kind":"date","page":"wrong-value","ocr":"","pdf":"a","note":"a"}]}`, "schema:items[0].page"},
		{`{"same_document":true,"pages":10,"items":[{"kind":"date","page":1,"ocr":"","pdf":"a","note":null}]}`, "schema:items[0].note:null"},
		{`{"same_document":true,"pages":10,"items":[{"kind":"date","page":1,"ocr":"","pdf":"a","note":"a","note":"wrong-value"}]}`, "schema:items[0].note:duplicate"},
		{`{"same_document":true,"pages":10,"items":[],"Pages":10}`, "schema:unknown_field"},
		{`{"same_document":true,"pages":10,"items":[]} {}`, "schema:trailing_data"},
	} {
		r, s, c, d := fixture(t)
		c.Response.Text = tc.body
		if err := run(t, r, d, options()); err != nil {
			t.Fatal(err)
		}
		if s.attempts[0].Reason != tc.reason {
			t.Fatalf("reason=%s want=%s", s.attempts[0].Reason, tc.reason)
		}
	}
}
func TestCheck_VerifiesQuoteBeforeDroppingEqual(t *testing.T) {
	r, s, c, d := fixture(t)
	c.Response.Text = `{"same_document":true,"pages":10,"items":[{"kind":"amount","page":1,"ocr":"invented","pdf":"invented","note":"equal"}]}`
	if err := run(t, r, d, options()); err != nil {
		t.Fatal(err)
	}
	if len(s.checks) != 0 || s.attempts[0].Reason != "ocr_quote_not_found" {
		t.Fatal("unverifiable equal quote accepted")
	}
}
