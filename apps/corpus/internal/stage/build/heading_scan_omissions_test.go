package build

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/source"

	"github.com/jackc/pgx/v5"
)

type headingScanReader struct {
	documents  []summaryDocument
	provisions [][]any
	gaps       [][]any
}

func (r headingScanReader) Query(_ context.Context, query string, _ ...any) (pgx.Rows, error) {
	rows := &summaryRows{}
	switch {
	case strings.Contains(query, "FROM document_summary"):
		for _, d := range r.documents {
			b, err := json.Marshal(d.Payload)
			if err != nil {
				return nil, err
			}
			rows.values = append(rows.values, []any{d.Key, b})
		}
	case strings.Contains(query, "FROM coverage_gap"):
		rows.values = r.gaps
	case strings.Contains(query, "FROM document d JOIN provision p"):
		rows.values = r.provisions
	default:
		return nil, fmt.Errorf("unexpected heading scan query: %s", query)
	}
	return rows, nil
}

type headingScanOutput struct {
	Items []struct {
		Anchor string `json:"anchor"`
	} `json:"items"`
	Omitted map[string]struct {
		Count   int `json:"count"`
		Longest []struct {
			Key    string `json:"key"`
			Anchor string `json:"anchor"`
			Length int    `json:"length"`
		} `json:"longest"`
	} `json:"omitted"`
	ShapeCounts map[string]int `json:"shape_breakdown"`
}

func TestHeadingScan_ReasonReport(t *testing.T) {
	r := headingScanReader{}
	r.provisions = append(r.provisions, []any{"broken", "source_tree", "chuong", "III", "chapter", "Bad."})
	for i := 0; i < 25; i++ {
		r.provisions = append(r.provisions, []any{"broken", "source_tree", "dieu", "", fmt.Sprintf("s%02d", i), strings.Repeat("S", 301+i) + "."})
		r.provisions = append(r.provisions, []any{"valid", "source_tree", "dieu", "", fmt.Sprintf("l%02d", i), strings.Repeat("L", 301+i) + "."})
		r.provisions = append(r.provisions, []any{"valid", "source_tree", "dieu", "", fmt.Sprintf("h%02d", i), strings.Repeat("H", i+1) + "."})
	}
	r.provisions = append(r.provisions,
		[]any{"valid", "source_tree", "dieu", "", "internal", "Private. text"},
		[]any{"valid", "source_tree", "dieu", "", "leading", "- Private"},
		[]any{"valid", "source_tree", "dieu", "", "multiple", "- Private. text!"},
		[]any{"valid", "source_tree", "dieu", "", "ok", "Private"},
		[]any{"ocr", "ocr", "dieu", "", "skip", "Private."},
		[]any{"gap", "source_tree", "dieu", "", "skip", "Private."},
	)
	r.gaps = [][]any{{"gap", "tree_text_partial"}}
	b, err := HeadingScan(context.Background(), r, "vn", "20990101_1", nil)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err = json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	if _, ok := raw["rule_3_omitted"]; ok {
		t.Fatal("obsolete report present")
	}
	var got headingScanOutput
	if err = json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	for reason, count := range map[string]int{"sequence": 26, "length": 25, "shape": 28} {
		report := got.Omitted[reason]
		if report.Count != count || len(report.Longest) != 20 {
			t.Errorf("%s: %+v", reason, report)
		}
		for i := 1; i < len(report.Longest); i++ {
			if report.Longest[i-1].Length < report.Longest[i].Length {
				t.Errorf("%s not sorted", reason)
			}
		}
	}
	if got.ShapeCounts["end_mark"] != 26 || got.ShapeCounts["internal_break"] != 1 || got.ShapeCounts["leading_marker"] != 1 {
		t.Fatalf("shape counts: %+v", got.ShapeCounts)
	}
	if strings.Contains(string(raw["omitted"]), "Private") || strings.Contains(string(raw["omitted"]), "heading") {
		t.Fatalf("text in report: %s", raw["omitted"])
	}
}

func TestHeadingScan_ExclusionsAndTies(t *testing.T) {
	r := headingScanReader{provisions: [][]any{
		{"doc", "source_tree", "dieu", "", "z", "Title."},
		{"doc", "source_tree", "dieu", "", "a", "Other."},
		{"doc", "source_tree", "dieu", "", "excluded", "Title."},
		{"doc", "source_tree", "phan", "", "part", "Title."},
		{"doc", "source_tree", "dieu", "", "empty", ""},
	}}
	b, err := HeadingScan(context.Background(), r, "vn", "20990101_1", []source.HeadingExclusion{{Key: "doc", Anchor: "excluded"}})
	if err != nil {
		t.Fatal(err)
	}
	var got headingScanOutput
	if err = json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	report := got.Omitted["shape"]
	if report.Count != 2 || len(report.Longest) != 2 || report.Longest[0].Anchor != "a" || report.Longest[1].Anchor != "z" {
		t.Fatalf("report: %+v", report)
	}
}

func TestHeadingScan_LongestMembershipAndDeterminism(t *testing.T) {
	omissions := []headingOmission{}
	for _, reason := range []string{"sequence", "length", "shape"} {
		for i := 0; i < 25; i++ {
			omissions = append(omissions, headingOmission{Key: "m", Anchor: fmt.Sprintf("%s-%02d", reason, i), Length: 100 + i, reason: reason})
		}
		for _, item := range [][2]string{{"a", "z"}, {"b", "a"}, {"a", "a"}} {
			omissions = append(omissions, headingOmission{Key: item[0], Anchor: item[1], Length: 124, reason: reason})
		}
	}
	b, err := encodeHeadingScan("vn", "20990101_1", nil, omissions)
	if err != nil {
		t.Fatal(err)
	}
	var got headingScanOutput
	if err = json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	for _, reason := range []string{"sequence", "length", "shape"} {
		items := got.Omitted[reason].Longest
		if len(items) != 20 || got.Omitted[reason].Count != 28 {
			t.Fatalf("%s: %+v", reason, got.Omitted[reason])
		}
		for i, item := range items {
			key, anchor, length := "m", fmt.Sprintf("%s-%02d", reason, 27-i), 127-i
			if i < 3 {
				key, anchor, length = [3]string{"a", "a", "b"}[i], [3]string{"a", "z", "a"}[i], 124
			}
			if item.Key != key || item.Anchor != anchor || item.Length != length {
				t.Errorf("%s[%d]=%+v want %s/%s/%d", reason, i, item, key, anchor, length)
			}
		}
	}
	for i, j := 0, len(omissions)-1; i < j; i, j = i+1, j-1 {
		omissions[i], omissions[j] = omissions[j], omissions[i]
	}
	again, err := encodeHeadingScan("vn", "20990101_1", nil, omissions)
	if err != nil || string(b) != string(again) {
		t.Fatalf("output changed with input order: %v", err)
	}
}

func TestHeadingScan_LengthAfterTrim(t *testing.T) {
	r := headingScanReader{provisions: [][]any{{"doc", "source_tree", "dieu", "", "at-cap", " " + strings.Repeat("a", 300) + " "}}}
	b, err := HeadingScan(context.Background(), r, "vn", "20990101_1", nil)
	if err != nil {
		t.Fatal(err)
	}
	var got headingScanOutput
	if err = json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.Omitted["length"].Count != 0 {
		t.Fatalf("trimmed 300 character heading reported: %s", b)
	}
}

func TestHeadingScan_PaddingDoesNotAffectLongest(t *testing.T) {
	r := headingScanReader{provisions: [][]any{{"doc", "source_tree", "dieu", "", "padded", strings.Repeat(" ", 40) + "x." + "\u3000"}}}
	for i := 1; i <= 20; i++ {
		r.provisions = append(r.provisions, []any{"doc", "source_tree", "dieu", "", fmt.Sprintf("a%02d", i), strings.Repeat("ố", i+1) + "."})
	}
	b, err := HeadingScan(context.Background(), r, "vn", "20990101_1", nil)
	if err != nil {
		t.Fatal(err)
	}
	var got headingScanOutput
	if err = json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	report := got.Omitted["shape"]
	if report.Count != 21 || len(report.Longest) != 20 {
		t.Fatalf("report=%+v", report)
	}
	for i, item := range report.Longest {
		if want := fmt.Sprintf("a%02d", 20-i); item.Anchor != want || item.Length != 22-i {
			t.Errorf("item %d=%+v, want %s length %d", i, item, want, 22-i)
		}
	}
}
