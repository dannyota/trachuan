package validity

import (
	"os"
	"strings"
	"testing"
)

func fixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
func article(s string) string { return "Điều 1. Hiệu lực thi hành\n" + s }
func TestParse_Dieu37TwoWholeOnePartial(t *testing.T) {
	r := Parse("Nghị định", fixture(t, "52-2024-dieu-37.txt"))
	if !r.Usable || r.Commencement != (Date{2024, 7, 1}) || len(r.Items) != 3 {
		t.Fatalf("result: %+v", r)
	}
	for i, want := range []struct {
		verb, scope, number string
		date                Date
		issuer              string
	}{{"replaces", "whole", "101/2012/NĐ-CP", Date{2012, 11, 22}, "Chính phủ về thanh toán không dùng tiền mặt"}, {"replaces", "whole", "80/2016/NĐ-CP", Date{2016, 7, 1}, "Chính phủ sửa đổi, bổ sung một số điều của Nghị định số 101/2012/NĐ-CP ngày 22 tháng 11 năm 2012 của Chính phủ về thanh toán không dùng tiền mặt"}, {"repeals", "partial", "", Date{}, ""}} {
		v := r.Items[i]
		if v.Verb != want.verb || v.Scope != want.scope || v.Number != want.number || v.IssueDate != want.date || v.Issuer != want.issuer || v.Sentence == "" {
			t.Errorf("item %d: %+v", i, v)
		}
	}
	if !strings.Contains(r.Items[2].Sentence, "16/2019/NĐ-CP") {
		t.Fatal(r.Items[2])
	}
}
func TestParse_Dieu34TruExceptionUnusable(t *testing.T) {
	r := Parse("Luật", fixture(t, "134-2025-dieu-34.txt"))
	if r.Usable || r.Commencement != (Date{2026, 3, 1}) || !hasReason(r, "exception") {
		t.Fatalf("%+v", r)
	}
}
func hasReason(r Result, kind string) bool {
	for _, v := range r.Reasons {
		if v.Kind == kind {
			return true
		}
	}
	return false
}
func TestParse_SpelledAndNumericDates(t *testing.T) {
	for _, date := range []string{"01 tháng 7 năm 2024", "1/7/2024"} {
		t.Run(date, func(t *testing.T) {
			r := Parse("Luật", article("Luật này có hiệu lực kể từ ngày "+date+"."))
			if !r.Usable || r.Commencement != (Date{2024, 7, 1}) {
				t.Fatalf("%+v", r)
			}
		})
	}
}
func TestParse_QuotedTextIgnored(t *testing.T) {
	r := Parse("Luật", article("Luật này có hiệu lực từ ngày 1/7/2024. Nội dung \"bãi bỏ\" được nhắc đến."))
	if !r.Usable {
		t.Fatalf("%+v", r)
	}
}
func TestParse_ClauseScopedDateMakesArticleUnusable(t *testing.T) {
	r := Parse("Luật", article("Luật này có hiệu lực từ ngày 1/7/2024. Khoản 2 Điều này có hiệu lực từ ngày 1/8/2024."))
	if r.Usable || !hasReason(r, "clause_scoped_commencement") {
		t.Fatalf("%+v", r)
	}
}
func TestParse_UnparsedEffectSentenceUnresolved(t *testing.T) {
	for _, s := range []string{"Luật này có hiệu lực kể từ ngày ký.", "Các văn bản trước đây hết hiệu lực kể từ ngày Luật này có hiệu lực."} {
		t.Run(s, func(t *testing.T) {
			r := Parse("Luật", article(s))
			if r.Usable || !hasReason(r, "unparsed_effect_sentence") {
				t.Fatalf("%+v", r)
			}
		})
	}
}
func TestParse_ListContinuationInheritsSubject(t *testing.T) {
	r := Parse("Luật", article("Luật này có hiệu lực từ ngày 1/7/2024. Luật này thay thế Luật số 1/2020/QH14 ngày 1/1/2020; Luật số 2/2021/QH14 ngày 2/2/2021, Luật số 3/2022/QH15 ngày 3/3/2022 và Luật số 4/2023/QH15 ngày 4/4/2023."))
	if !r.Usable || len(r.Items) != 4 {
		t.Fatalf("%+v", r)
	}
}
func TestParse_PartialItemPrefixes(t *testing.T) {
	for _, p := range []string{"Điều 3", "khoản 2", "điểm a", "Chương II", "Mục 1", "Phụ lục I", "cụm từ này"} {
		t.Run(p, func(t *testing.T) {
			r := Parse("Luật", article("Luật này có hiệu lực từ ngày 1/7/2024. Luật này bãi bỏ "+p+" của Luật số 1/2020/QH14."))
			if !r.Usable || len(r.Items) != 1 || r.Items[0].Scope != "partial" {
				t.Fatalf("%+v", r)
			}
		})
	}
}
func TestParse_SubjectTypeMustMatchDocument(t *testing.T) {
	r := Parse("Luật", article("Nghị định này có hiệu lực từ ngày 1/7/2024."))
	if r.Usable || !hasReason(r, "unparsed_effect_sentence") {
		t.Fatalf("%+v", r)
	}
}
func TestParse_InvalidCalendarDateRejected(t *testing.T) {
	r := Parse("Luật", article("Luật này có hiệu lực từ ngày 31/2/2024."))
	if r.Usable || !hasReason(r, "unparsed_effect_sentence") {
		t.Fatalf("%+v", r)
	}
}
func TestSelect_EffectArticleHeadings(t *testing.T) {
	for _, tc := range []struct {
		s    string
		want bool
	}{{"Hiệu lực thi hành", true}, {"ĐIỀU KHOẢN THI HÀNH", true}, {"Hiệu lực thi hành", true}, {"Hiệu lực", false}} {
		if got := Select(tc.s); got != tc.want {
			t.Errorf("%q: %v", tc.s, got)
		}
	}
}

func TestParse_OtherUnresolvedForms(t *testing.T) {
	for _, tc := range []struct{ name, text, reason string }{
		{"multiple dates", "Luật này có hiệu lực từ ngày 1/1/2024. Luật này có hiệu lực từ ngày 1/2/2024.", "multiple_commencement_sentences"},
		{"provision date", "Luật này có hiệu lực từ ngày 1/1/2024. Điều 2 có hiệu lực từ ngày 1/2/2024.", "clause_scoped_commencement"},
		{"missing terminator", "Luật này có hiệu lực từ ngày 1/1/2024", "unparsed_effect_sentence"},
		{"unparsed effect inside item", "Luật này có hiệu lực từ ngày 1/1/2024. Luật này bãi bỏ Điều 3 và hết hiệu lực từ ngày 1/2/2024.", "unparsed_effect_sentence"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := Parse("Luật", article(tc.text))
			if r.Usable || !hasReason(r, tc.reason) {
				t.Fatalf("%+v", r)
			}
		})
	}
}

func TestParse_QuoteWithPeriod(t *testing.T) {
	r := Parse("Luật", article("Luật này có hiệu lực từ ngày 1/1/2024. Nội dung “A. bãi bỏ” được nhắc đến."))
	if !r.Usable {
		t.Fatalf("%+v", r)
	}
}

func TestParse_WholeItemRequiresIssueDate(t *testing.T) {
	r := Parse("Luật", article("Luật này có hiệu lực từ ngày 1/1/2024. Luật này thay thế Luật số 1/2020/QH14."))
	if r.Usable || !hasReason(r, "unparsed_effect_sentence") {
		t.Fatalf("%+v", r)
	}
}

func TestParse_ReplacementWithExceptionUnresolved(t *testing.T) {
	// Refusing "khấu trừ" is a deliberate cost of failing closed on "trừ".
	for _, q := range []string{"trừ Điều 3", "ngoại trừ Điều 3", "không bao gồm Điều 3", "khấu trừ thuế"} {
		t.Run(q, func(t *testing.T) {
			r := Parse("Nghị định", article("Nghị định này có hiệu lực từ ngày 1/7/2024. Nghị định này thay thế Nghị định số 1/2020/NĐ-CP ngày 1/1/2020 của Chính phủ, "+q+"."))
			if r.Usable {
				t.Fatalf("qualified replacement accepted: %+v", r)
			}
		})
	}
}

func TestParse_SpacedAndWrappedClauseDateUnusable(t *testing.T) {
	for _, gap := range []string{"  ", "\n", "\u00a0"} {
		r := Parse("Luật", article("Luật này có hiệu lực từ ngày 1/7/2024. Khoản 2 có"+gap+"hiệu lực từ ngày 1/1/2025."))
		if r.Usable {
			t.Fatalf("gap %q accepted: %+v", gap, r)
		}
	}
}

func TestParse_ReplacementTitleWithDoiVoiAccepted(t *testing.T) {
	r := Parse("Nghị định", article("Nghị định này có hiệu lực từ ngày 1/7/2024. Nghị định này thay thế Nghị định số 1/2020/NĐ-CP ngày 1/1/2020 của Chính phủ quy định đối với thanh toán."))
	if !r.Usable {
		t.Fatal(r)
	}
}
