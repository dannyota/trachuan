package build

import (
	"context"
	"crypto/sha256"
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
		Key     string `json:"key"`
		Anchor  string `json:"anchor"`
		Heading string `json:"heading"`
	} `json:"items"`
	Omitted struct {
		Count   int `json:"count"`
		Longest []struct {
			Key    string `json:"key"`
			Anchor string `json:"anchor"`
			Length int    `json:"length"`
		} `json:"longest"`
	} `json:"rule_3_omitted"`
}

func TestHeadingScan_RuleThreeBoundaryAndEligibility(t *testing.T) {
	d := summaryFixture()
	d.Payload.Outline = []summaryOutline{{Kind: "dieu", Anchor: "at-cap", Heading: strings.Repeat("ố", 300)}}
	r := headingScanReader{documents: []summaryDocument{d}, provisions: [][]any{
		{d.Key, "source_tree", "dieu", "at-cap", 300},
		{d.Key, "source_tree", "dieu", "over-cap", 301},
		{d.Key, "source_tree", "chuong", "chapter", 302},
		{d.Key, "source_tree", "dieu", "excluded", 500},
		{d.Key, "source_tree", "phan", "part", 400},
		{d.Key, "source_tree", "muc", "section", 400},
		{"vn:vbpl:2", "ocr", "dieu", "ocr", 400},
		{"vn:vbpl:3", "source_tree", "dieu", "structural", 400},
	}, gaps: [][]any{{"vn:vbpl:3", "tree_text_partial"}}}
	b, err := HeadingScan(context.Background(), r, "vn", "20990101_1", []source.HeadingExclusion{{Key: d.Key, Anchor: "excluded"}})
	if err != nil {
		t.Fatal(err)
	}
	var got headingScanOutput
	if err = json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Items) != 1 || got.Items[0].Anchor != "at-cap" || got.Items[0].Heading != strings.Repeat("ố", 300) {
		t.Fatalf("items=%+v", got.Items)
	}
	if got.Omitted.Count != 2 || len(got.Omitted.Longest) != 2 || got.Omitted.Longest[0].Anchor != "chapter" || got.Omitted.Longest[0].Length != 302 || got.Omitted.Longest[1].Anchor != "over-cap" || got.Omitted.Longest[1].Length != 301 {
		t.Fatalf("omitted=%+v", got.Omitted)
	}
	var raw map[string]json.RawMessage
	if err = json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw["rule_3_omitted"]), "heading") || strings.Contains(string(raw["rule_3_omitted"]), "ố") {
		t.Fatalf("heading text in omissions: %s", raw["rule_3_omitted"])
	}
}

func TestHeadingScan_RuleThreeOrderLimitAndNoSummary(t *testing.T) {
	r := headingScanReader{gaps: [][]any{{"vn:vbpl:10", "summary_field_capped"}, {"vn:vbpl:2", "summary_row_capped"}}}
	for i := 24; i >= 0; i-- {
		r.provisions = append(r.provisions, []any{"vn:vbpl:2", "source_tree", "dieu", fmt.Sprintf("a%02d", i), 301 + i})
	}
	r.provisions = append(r.provisions,
		[]any{"vn:vbpl:10", "source_tree", "chuong", "z", 325},
		[]any{"vn:vbpl:10", "source_tree", "dieu", "a", 325},
	)
	b, err := HeadingScan(context.Background(), r, "vn", "20990101_1", nil)
	if err != nil {
		t.Fatal(err)
	}
	var got headingScanOutput
	if err = json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Items) != 0 || got.Omitted.Count != 27 || len(got.Omitted.Longest) != 20 {
		t.Fatalf("items=%d omitted=%+v", len(got.Items), got.Omitted)
	}
	for i, item := range got.Omitted.Longest {
		key, anchor, length := "vn:vbpl:2", fmt.Sprintf("a%02d", 26-i), 327-i
		if i < 2 {
			key, anchor, length = "vn:vbpl:10", "a", 325
			if i == 1 {
				anchor = "z"
			}
		}
		if item.Key != key || item.Anchor != anchor || item.Length != length {
			t.Fatalf("longest[%d]=%+v, want %s/%s/%d", i, item, key, anchor, length)
		}
	}
	for i, j := 0, len(r.provisions)-1; i < j; i, j = i+1, j-1 {
		r.provisions[i], r.provisions[j] = r.provisions[j], r.provisions[i]
	}
	again, err := HeadingScan(context.Background(), r, "vn", "20990101_1", nil)
	if err != nil || string(again) != string(b) {
		t.Fatalf("nondeterministic scan: %v", err)
	}
	r.provisions = append(r.provisions, []any{"vn:vbpl:2", "source_tree", "dieu", "short", 301})
	changed, err := HeadingScan(context.Background(), r, "vn", "20990101_1", nil)
	if err != nil || sha256.Sum256(changed) == sha256.Sum256(b) {
		t.Fatalf("omission count absent from scan hash: %v", err)
	}
}
