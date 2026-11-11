package textparse

import (
	"os"
	"strings"
	"testing"
)

func TestParse_StructureGrammar(t *testing.T) {
	r := Parse(paragraphs(long("Điều 1. Một\nChương này đối với nội dung\nĐiều 2. Hai")))
	for _, p := range r.Provisions {
		if p.Kind == "dieu" && p.Number == "1" && strings.Contains(p.Text, "Chương này đối với") {
			return
		}
	}
	t.Fatalf("article 1 lost running text: %+v", r.Provisions)
}
func TestParse_ScopedStructureAnchors(t *testing.T) {
	r := Parse(paragraphs(long("Chương I\nĐiều 1. Một\nQUY CHẾ\nChương I\nĐiều 1. Hai\nĐiều 3. Ba")))
	seen := map[string]bool{}
	for _, p := range r.Provisions {
		if seen[p.Anchor] {
			t.Fatalf("duplicate anchor %s", p.Anchor)
		}
		seen[p.Anchor] = true
		if p.ScopeIndex == 1 && p.Kind != "other" {
			t.Fatalf("failed scope asserts structure: %+v", p)
		}
	}
	r = Parse(paragraphs(long("Điều 1. Một\nChương I\nĐiều 2. Hai\nChương I\nĐiều 3. Ba")))
	if r.Scopes[0].Accepted {
		t.Fatal("duplicate structure accepted")
	}
}
func TestParse_LeadingStructureInMainScope(t *testing.T) {
	r := Parse(paragraphs(long("Chương I\nQUY ĐỊNH CHUNG\nĐiều 1. Một")))
	for _, p := range r.Provisions {
		if p.Kind == "chuong" {
			if p.ScopeIndex != 0 {
				t.Fatalf("scope %d", p.ScopeIndex)
			}
			return
		}
	}
	t.Fatal("missing chapter")
}
func TestParse_AppendixRunningReferences(t *testing.T) {
	for _, s := range []string{"Phụ lục này là một phần của văn bản.", "Phụ lục 1 của văn bản này được áp dụng.", "Mẫu số 1 được nêu trong quy định này.", "Phụ lục số 01 này được áp dụng.", "Mẫu số II nêu trong quy định này.", "Phụ lục số này quy định nội dung."} {
		r := Parse(paragraphs(long("Điều 1. Một\n" + s + "\nĐiều 2. Hai")))
		if len(r.Scopes) != 1 || !r.Scopes[0].Accepted {
			t.Fatalf("%q: %+v", s, r.Scopes)
		}
	}
}
func TestParse_AppendixTitles(t *testing.T) {
	for _, title := range []string{"Phụ lục số 01", "Phụ lục 01a", "Phụ lục IV", "Phụ lục số IVa", "Mẫu số 01a", "Mẫu số IIb"} {
		t.Run(title, func(t *testing.T) {
			r := Parse(paragraphs(long("Điều 1. Một\n" + title + "\nĐiều 1. Mẫu")))
			if len(r.Scopes) != 2 || r.Scopes[1].Kind != "tail" {
				t.Fatalf("scopes: %+v", r.Scopes)
			}
			foundTitle := false
			for _, p := range r.Provisions {
				if p.ScopeIndex == 1 && strings.HasPrefix(p.Text, title) {
					foundTitle = true
				}
				if p.ScopeIndex == 1 && p.Kind != "other" {
					t.Fatalf("tail provision: %+v", p)
				}
			}
			if !foundTitle {
				t.Fatal("appendix title did not start tail")
			}
		})
	}
}
func TestParse_InvalidHeadingCandidate(t *testing.T) {
	for _, s := range []string{"Điều 2: Phạm vi", "Điều 2 Phạm vi", "Điều 2Phạm vi", "Điều 2"} {
		r := Parse(paragraphs(long("Điều 1. Một\n" + s)))
		if r.Scopes[0].Accepted || !contains(r.Gaps, "structure_uncertain") {
			t.Fatalf("%q: %+v", s, r.Scopes)
		}
	}
}
func TestParse_QuoteAcrossParagraphs(t *testing.T) {
	r := Parse(paragraphs(long("Điều 1. Một\n“Điều 1. Trích\nĐiều 2. Trích tiếp\nKết thúc.”\nĐiều 2. Hai")))
	var articles []string
	for _, p := range r.Provisions {
		if p.Kind == "dieu" {
			articles = append(articles, p.Number)
		}
	}
	if strings.Join(articles, ",") != "1,2" || !r.Scopes[0].Accepted {
		t.Fatalf("articles %v scopes %+v", articles, r.Scopes)
	}
	r = Parse(paragraphs(long("Điều 1. Một\n“Trích dẫn chưa đóng")))
	if r.Scopes[0].Accepted {
		t.Fatal("unclosed quote accepted")
	}
}
func TestParse_DecreeArticle21Boundary(t *testing.T) {
	b, err := os.ReadFile("testdata/ocr_decree_142_2026.txt")
	if err != nil {
		t.Fatal(err)
	}
	r := Parse(paragraphs(string(b)))
	for _, p := range r.Provisions {
		if p.Kind == "dieu" && p.Number == "21" {
			if !strings.Contains(p.Text, "Chương này đối với") {
				t.Fatal("article 21 truncated before running reference")
			}
			return
		}
	}
	t.Fatal("article 21 missing")
}

func TestParse_QuoteStartingAtCapitalTitle(t *testing.T) {
	r := Parse(paragraphs(long("Chương I\n“QUY ĐỊNH CHUNG”\nĐiều 1. Một")))
	for _, p := range r.Provisions {
		if p.Kind == "chuong" {
			if p.Text != "Chương I\n“QUY ĐỊNH CHUNG”" {
				t.Fatalf("chapter title %q", p.Text)
			}
			return
		}
	}
	t.Fatal("missing chapter")
}

func TestParse_HeadingFormsAndScopes(t *testing.T) {
	for _, tc := range []struct{ line, kind string }{
		{"Phần thứ nhất", "phan"}, {"Phần II: Quy định", "phan"}, {"Chương IV. Quy định", "chuong"}, {"Chương 2: Quy định", "chuong"}, {"Mục 1 - Quy định", ""}, {"Mục 1. Quy định", "muc"}, {"Chương này quy định", ""}, {"Mục tiêu", ""},
	} {
		kind, _, ok := structure(tc.line)
		if ok != (tc.kind != "") || kind != tc.kind {
			t.Errorf("%q: kind=%q ok=%v", tc.line, kind, ok)
		}
	}
	r := Parse(paragraphs(long("Chương I\nĐiều 1. Một\nQUY CHẾ\nChương I\nĐiều 1. Hai")))
	found := false
	for _, p := range r.Provisions {
		if p.ScopeIndex == 1 && p.Kind == "chuong" {
			found = true
			if p.Anchor != "kem-theo-1/chuong-i" {
				t.Fatalf("anchor %q", p.Anchor)
			}
		}
	}
	if !found {
		t.Fatal("missing instrument chapter")
	}
}

func TestParse_QuoteHidesScopeBoundary(t *testing.T) {
	r := Parse(paragraphs(long("Điều 1. Một\n“Trích dẫn\nNơi nhận:\nKết thúc.”\nĐiều 2. Hai")))
	if len(r.Scopes) != 1 || !r.Scopes[0].Accepted {
		t.Fatalf("scopes %+v", r.Scopes)
	}
}

func TestParse_MismatchedQuoteStylesFailScope(t *testing.T) {
	r := Parse(paragraphs(long("Điều 1. Một\n“Trích dẫn\nKết thúc.\"\nĐiều 2. Hai")))
	if len(r.Scopes) != 1 || r.Scopes[0].Accepted || !contains(r.Gaps, "structure_uncertain") {
		t.Fatalf("scopes %+v gaps %v", r.Scopes, r.Gaps)
	}
}
