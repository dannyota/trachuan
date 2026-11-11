package textparse

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestParse_NumberTokenBoundaries(t *testing.T) {
	for _, s := range []string{"Điều 2Phạm vi", "Điều 2phạm vi"} {
		t.Run(s, func(t *testing.T) {
			r := Parse(paragraphs(long("Điều 1. Một\n" + s)))
			if len(r.Scopes) != 1 || r.Scopes[0].Accepted || !contains(r.Gaps, "structure_uncertain") {
				t.Fatalf("scopes=%+v gaps=%v", r.Scopes, r.Gaps)
			}
		})
	}
	r := Parse(paragraphs(long("Điều 1. Một\nĐiều 2. Hai\nĐiều 3. Ba\nĐiều 3a. Tên\nĐiều 3a của Luật này\nĐiều 4. Bốn")))
	if len(r.Scopes) != 1 || !r.Scopes[0].Accepted {
		t.Fatalf("scopes=%+v", r.Scopes)
	}
	var found bool
	for _, p := range r.Provisions {
		if p.Kind == "dieu" && p.Number == "3a" {
			found = true
			if !strings.Contains(p.Text, "Điều 3a của Luật") {
				t.Fatal("running reference missing")
			}
		}
	}
	if !found {
		t.Fatal("missing inserted article")
	}
}
func TestParse_IndependentQuotationStyles(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"curly outer", "“Trích dẫn \"nội dung\nĐiều 2. Trích dẫn\nkết thúc\".”"},
		{"straight outer", "\"Trích dẫn “nội dung\nĐiều 2. Trích dẫn\nkết thúc”.\""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := Parse(paragraphs(long("Điều 1. Một\n" + tc.body + "\nĐiều 2. Hai")))
			if len(r.Scopes) != 1 || !r.Scopes[0].Accepted {
				t.Fatalf("scopes=%+v", r.Scopes)
			}
			for _, p := range r.Provisions {
				if p.Kind == "dieu" && p.Number == "1" && !strings.Contains(p.Text, "Điều 2. Trích dẫn") {
					t.Fatal("quoted article escaped")
				}
			}
		})
	}
	r := Parse(paragraphs(long("Điều 1. Một\n“Đã đóng” rồi mở \"")))
	if len(r.Scopes) != 1 || r.Scopes[0].Accepted || !contains(r.Gaps, "structure_uncertain") {
		t.Fatalf("scopes=%+v gaps=%v", r.Scopes, r.Gaps)
	}
}
func TestParse_QuotationFailuresBounded(t *testing.T) {
	const marks = 200000
	p := paragraphs("Điều 1. Một\n" + strings.Repeat("”", marks))
	_, failures := quotedParagraphs(p)
	if len(failures) != 1 {
		t.Fatalf("failures=%d want 1", len(failures))
	}
	var b strings.Builder
	b.WriteString("Điều 1. Một\n")
	b.WriteString(strings.Repeat("”", marks))
	b.WriteByte('\n')
	for i := 0; i < 2000; i++ {
		fmt.Fprint(&b, "QUY CHẾ\nĐiều 1. Một\n")
	}
	start := time.Now()
	r := Parse(paragraphs(b.String()))
	if elapsed := time.Since(start); elapsed >= time.Second {
		t.Fatalf("parse took %s", elapsed)
	}
	if len(r.Scopes) != 2001 || r.Scopes[0].Accepted {
		t.Fatalf("scopes=%d", len(r.Scopes))
	}
}
