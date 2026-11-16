package build

import (
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/validity"
)

func TestRuleA_ChinhphuIssueDate(t *testing.T) {
	article := validity.Parse("Nghị định", "Điều 1. Hiệu lực thi hành\nNghị định này có hiệu lực thi hành từ ngày 01 tháng 5 năm 2026.")
	if !article.Usable {
		t.Fatalf("article = %+v", article)
	}
	for _, tc := range []struct {
		name, raw, class, basis, from string
		current                       bool
	}{
		{"converted", "2026-04-30", "in_force", "derived_from_text", "2026-05-01", true},
		{"old raw", "\n 30-04-2026", "unknown", "source_status", "2026-05-01", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parsed, err := ParseDate(tc.raw)
			d := testDoc("vn:chinhphu:218029", "142/2026/NĐ-CP", dateString(parsed), "2026-05-01")
			d.statusCode = ""
			d.sourceClass = "unknown"
			d.dateGap = err != nil
			d.article = &article
			got := runTest(t, []validityDocument{d}, nil, nil).documents[d.key].values
			if got.Class != tc.class || got.Current != tc.current || got.Basis != tc.basis || dateValue(got.From) != tc.from {
				t.Fatalf("result = %+v", got)
			}
		})
	}
}
