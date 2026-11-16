package build

import (
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/validity"
)

func TestValidity_ChinhphuOCRAndCorrections(t *testing.T) {
	for _, tc := range []struct {
		name, key, number, rawIssue, rawFrom, asOf, anchor string
		correction                                         bool
		issue, class, basis, from                          string
		current                                            bool
	}{
		{"without correction", "vn:chinhphu:218029", "142/2026/NĐ-CP", "2026-04-30", "2026-05-01", "2026-11-16", "dieu-45", false, "2026-04-30", "unknown", "source_status", "2026-05-01", false},
		{"with correction", "vn:chinhphu:218029", "142/2026/NĐ-CP", "2026-04-30", "2026-05-01", "2026-11-16", "dieu-45", true, "2026-04-30", "in_force", "manual_correction", "2026-05-01", true},
		{"old issue date", "vn:chinhphu:218029", "142/2026/NĐ-CP", "\n 30-04-2026", "2026-05-01", "2026-11-16", "dieu-45", false, "", "unknown", "source_status", "2026-05-01", false},
		{"future corrected", "vn:chinhphu:218658", "33/2026/QĐ-TTg", "2026-06-30", "2026-08-15", "2026-08-14", "doan-9", true, "2026-06-30", "in_force", "manual_correction", "2026-08-15", true},
		{"effective corrected", "vn:chinhphu:218658", "33/2026/QĐ-TTg", "2026-06-30", "2026-08-15", "2026-08-16", "doan-9", true, "2026-06-30", "in_force", "manual_correction", "2026-08-15", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			issue, issueErr := ParseDate(tc.rawIssue)
			from, fromErr := ParseDate(tc.rawFrom)
			if dateString(issue) != tc.issue || fromErr != nil || dateString(from) != tc.rawFrom || (issueErr != nil) != (tc.issue == "") {
				t.Fatalf("issue=%v issueErr=%v from=%v fromErr=%v", issue, issueErr, from, fromErr)
			}
			d := testDoc(tc.key, tc.number, dateString(issue), dateString(from))
			d.statusCode = ""
			d.sourceClass = "unknown"
			d.textSource = "ocr"
			d.anchor = tc.anchor
			d.dateGap = issueErr != nil
			docType, text := "Nghị định", "Nghị định này có hiệu lực thi hành từ ngày 01 tháng 5 năm 2026."
			if tc.key == "vn:chinhphu:218658" {
				docType, text = "Quyết định", "Quyết định này có hiệu lực thi hành từ ngày 15 tháng 8 năm 2026."
			}
			d.docType = docType
			article := validity.Parse(docType, "Điều 1. Hiệu lực thi hành\n"+text)
			if !article.Usable {
				t.Fatalf("article = %+v", article)
			}
			d.article = &article
			corrections := map[string]source.Correction{}
			if tc.correction {
				status, effective := "in_force", tc.rawFrom
				corrections[tc.key] = source.Correction{Key: tc.key, Number: tc.number, Set: source.CorrectionSet{StatusClass: &status, EffFrom: &effective}, Evidence: source.CorrectionEvidence{Anchor: tc.anchor}}
			}
			result, err := runValidity([]validityDocument{d}, nil, corrections, tc.asOf)
			if err != nil {
				t.Fatal(err)
			}
			got := result.documents[tc.key].values
			if got.Class != tc.class || got.Current != tc.current || got.Basis != tc.basis || dateValue(got.From) != tc.from || !hasReview(result, "parsed_text_article") {
				t.Fatalf("result = %+v reviews = %+v", got, result.reviews)
			}
		})
	}
}
