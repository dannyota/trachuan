package textparse

import (
	"strings"
	"testing"
)

func TestParse_QuotationDepth(t *testing.T) {
	for _, tc := range []struct{ name, input string }{
		{"nested", "Điều 1. Một\n“Bắt đầu «trong»\nĐiều 2. Trích dẫn\nKết thúc.”\nĐiều 2. Hai"},
		{"mid-paragraph", "Điều 1. Một\nSửa đổi như sau: “Điều 9. Nội dung\nĐiều 2. Trích dẫn\nKết thúc.”\nĐiều 2. Hai"},
		{"single marks ignored", "Điều 1. Một\n“Thay cụm từ ‘cũ’\nĐiều 2. Trích dẫn\nKết thúc.”\nĐiều 2. Hai"},
		{"straight", "Điều 1. Một\nSửa đổi: \"Điều 9. Nội dung\nĐiều 2. Trích dẫn\nKết thúc.\"\nĐiều 2. Hai"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := Parse(paragraphs(long(tc.input)))
			var articles []string
			for _, p := range r.Provisions {
				if p.Kind == "dieu" {
					articles = append(articles, p.Number)
				}
			}
			if len(r.Scopes) != 1 || !r.Scopes[0].Accepted || strings.Join(articles, ",") != "1,2" {
				t.Fatalf("scopes=%+v articles=%v", r.Scopes, articles)
			}
			for _, p := range r.Provisions {
				if p.Kind == "dieu" && p.Number == "1" && !strings.Contains(p.Text, "Điều 2. Trích dẫn") {
					t.Fatal("quote escaped article")
				}
			}
		})
	}
}
func TestParse_UnbalancedClosingQuoteFailsScope(t *testing.T) {
	r := Parse(paragraphs(long("Điều 1. Một\nNội dung đóng sai ”\nĐiều 2. Hai")))
	if len(r.Scopes) != 1 || r.Scopes[0].Accepted || !contains(r.Gaps, "structure_uncertain") {
		t.Fatalf("scopes=%+v gaps=%v", r.Scopes, r.Gaps)
	}
}
func TestParse_CandidatesByNegation(t *testing.T) {
	for _, s := range []string{"Điều 2 : Phạm vi", "Điều 2: Phạm vi", "Điều 2 Phạm vi", "Điều 2.phạm vi"} {
		t.Run(s, func(t *testing.T) {
			r := Parse(paragraphs(long("Điều 1. Một\n" + s)))
			if len(r.Scopes) != 1 || r.Scopes[0].Accepted {
				t.Fatalf("accepted %+v", r.Scopes)
			}
		})
	}
	for _, s := range []string{"Điều 6 của Luật", "Điều 6, Điều 7 của Luật", "Điều 6) nội dung"} {
		t.Run(s, func(t *testing.T) {
			r := Parse(paragraphs(long("Điều 1. Một\n" + s + "\nĐiều 2. Hai")))
			if len(r.Scopes) != 1 || !r.Scopes[0].Accepted {
				t.Fatalf("rejected %+v", r.Scopes)
			}
		})
	}
}
func TestParse_AppendixWholeParagraph(t *testing.T) {
	r := Parse(paragraphs(long("Điều 1. Một\nPhụ lục 1 của Thông tư này\nđược áp dụng cho trường hợp này\nĐiều 2. Hai")))
	if len(r.Scopes) != 1 || !r.Scopes[0].Accepted {
		t.Fatalf("scopes %+v", r.Scopes)
	}
	var texts []string
	for _, p := range r.Provisions {
		if p.Kind == "dieu" {
			texts = append(texts, p.Anchor)
			if p.Number == "1" && !strings.Contains(p.Text, "được áp dụng") {
				t.Fatal("article truncated")
			}
		}
	}
	if strings.Join(texts, ",") != "dieu-1,dieu-2" {
		t.Fatalf("anchors %v", texts)
	}
	for _, s := range []string{"Phụ lục số 01", "Phụ lục II.", "Mẫu số 02a:"} {
		t.Run(s, func(t *testing.T) {
			r := Parse(paragraphs(long("Điều 1. Một\n" + s + "\nVăn bản biểu mẫu")))
			if len(r.Scopes) != 2 || r.Scopes[1].Kind != "tail" {
				t.Fatalf("scopes %+v", r.Scopes)
			}
		})
	}
}
