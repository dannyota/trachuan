package retrieval

import (
	"reflect"
	"strings"
	"testing"
)

func TestValidateQuery_DiacriticsRequired(t *testing.T) {
	v, err := ValidateQuery("ngan hang")
	if err != nil || v.Reason != "diacritics_required" || v.Message != "Vui lòng nhập câu hỏi tiếng Việt có dấu hoặc số hiệu văn bản." {
		t.Fatalf("got %+v, %v", v, err)
	}
}
func TestValidateQuery_UnsupportedQuery(t *testing.T) {
	v, err := ValidateQuery("123 %")
	if err != nil || v.Reason != "unsupported_query" {
		t.Fatalf("got %+v, %v", v, err)
	}
}
func TestValidateQuery_DocumentNumberPassesWithoutMarks(t *testing.T) {
	v, err := ValidateQuery("50/2024/TT-NHNN")
	if err != nil || !v.Accepted || !v.Routed {
		t.Fatalf("got %+v, %v", v, err)
	}
}
func TestParseReferences_Grammar(t *testing.T) {
	for _, tc := range []struct {
		q    string
		want []string
	}{
		{"50/2024/TT", []string{"50/2024/TT"}},
		{"50 / 2024 / TT-NHNN, 21/VBHN-NHNN", []string{"50/2024/TT-NHNN", "21/VBHN-NHNN"}},
		{"2024 50/2024 % abc50/2024/TT", nil},
	} {
		got, err := ParseReferences(tc.q)
		if err != nil || !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("%q: %v, %v", tc.q, got, err)
		}
	}
}
func TestNormalizeNumber(t *testing.T) {
	if got := NormalizeNumber(" 01 / 2024 / đt - a "); got != "01/2024/DT-A" {
		t.Fatal(got)
	}
}
func TestRecipe1_HashStable(t *testing.T) {
	r := Recipe1()
	first, second := r.Hash(), Recipe1().Hash()
	if first == "" || first != second {
		t.Fatal("unstable hash")
	}
}

func TestParseReferences_BoundsAndLatinLetters(t *testing.T) {
	for _, q := range []string{strings.Repeat("1/A ", 9), "1/" + strings.Repeat("A", 127)} {
		if _, err := ParseReferences(q); err == nil {
			t.Fatalf("expected reference bound error")
		}
	}
	got, err := ParseReferences("21 / ĐT - NHNN")
	if err != nil || !reflect.DeepEqual(got, []string{"21/DT-NHNN"}) {
		t.Fatalf("got %v, %v", got, err)
	}
}

func TestValidateQuery_RejectsExcessRunes(t *testing.T) {
	if _, err := ValidateQuery(strings.Repeat("á", 2001)); err == nil {
		t.Fatal("accepted query above the rune limit")
	}
}

func TestRecipe_ValidityPolicyVersionBumped(t *testing.T) {
	// Recipe 2 changes the serialized version, cap and non-current tail.
	if got := Recipe1().Hash(); got != "108211da043b1de9c3da72f0306114af730b7d610cec2f5701e62307343e2922" {
		t.Errorf("hash = %q", got)
	}
	if got := Recipe1().Validity; got != "document-dates/3" {
		t.Errorf("validity = %q", got)
	}
}

func TestRecipe1_ContextScopeField(t *testing.T) {
	b, err := Recipe1().JSON()
	if err != nil || !strings.Contains(string(b), `"context_scope":"number-routed/1"`) {
		t.Fatalf("recipe = %s, error %v", b, err)
	}
}
