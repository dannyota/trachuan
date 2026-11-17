package build

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
)

func summaryFixture() summaryDocument {
	return summaryDocument{Key: "vn:vbpl:1", Class: "primary", TextSource: "source_tree", StatusClass: "unknown", StatusBasis: "source_status", Payload: summaryPayload{Number: "1", Title: "Title", Type: "Law", Issuer: "Issuer", Outline: []summaryOutline{{Kind: "chuong", Label: "Chương I", Anchor: "chuong-i", Heading: "General"}, {Kind: "dieu", Label: "Điều 1", Anchor: "dieu-1", Heading: "Scope"}}}}
}

func makeUnitSummary(d summaryDocument, gaps []InventoryGap, excluded map[string]bool) (summaryDocument, summaryCapStep, error) {
	return makeSummary(context.Background(), &summaryTx{sizes: []int{65536, 65536, 65536}}, d, gaps, excluded)
}

func TestSummary_EligibilityAndOutlineCap(t *testing.T) {
	for _, tc := range []struct {
		class           string
		count           int
		label           string
		indexed, capped bool
	}{
		{"primary", 1, "Điều 1", true, false}, {"relation_context", 1, "Điều 1", false, false}, {"secondary", 1, "Điều 1", false, false},
		{"primary", 0, "", false, false}, {"primary", 1, "", false, false}, {"primary", 2200, "x", true, false}, {"primary", 2201, "x", false, true},
	} {
		d := summaryFixture()
		d.Class = tc.class
		d.Payload.Outline = make([]summaryOutline, tc.count)
		for i := range d.Payload.Outline {
			d.Payload.Outline[i] = summaryOutline{Kind: "dieu", Label: tc.label}
		}
		r, step, err := makeUnitSummary(d, nil, nil)
		capped := step == summaryOutlineDropped
		if err != nil || r.Indexed != tc.indexed || capped != tc.capped {
			t.Fatalf("%+v: %+v %t %v", tc, r, capped, err)
		}
		if capped && len(r.Payload.Outline) != 0 {
			t.Fatal("capped outline retained")
		}
		if !capped && len(r.Payload.Outline) != tc.count {
			t.Fatalf("outline entries=%d, want %d", len(r.Payload.Outline), tc.count)
		}
	}
}

func TestSummary_HeadingRules(t *testing.T) {
	for _, source := range []string{"source_tree", "html_transcription", "pdf_text", "ocr", "none"} {
		d := summaryFixture()
		d.TextSource = source
		r, _, err := makeUnitSummary(d, nil, nil)
		if err != nil || (r.Payload.Outline[0].Heading != "") != (source == "source_tree") {
			t.Fatalf("%s: %+v %v", source, r, err)
		}
	}
	for _, kind := range []string{"ambiguous_tree_text", "article_number_out_of_range", "article_sequence", "structure_uncertain", "text_number_mismatch", "tree_headings_only", "tree_headings_partial", "tree_text_partial", "unknown_node_kind", "invalid_ancestry", "no_tree", "text_source_ambiguous"} {
		for _, stage := range []string{"fetch", "extract", "chunk", "build"} {
			r, _, err := makeUnitSummary(summaryFixture(), []InventoryGap{{Stage: stage, ItemKey: "vn:vbpl:1", Kind: kind}}, nil)
			if err != nil || r.Payload.Outline[0].Heading != "" || r.Payload.Outline[1].Heading != "" {
				t.Fatalf("%s/%s: %+v %v", stage, kind, r, err)
			}
		}
	}
	d := summaryFixture()
	d.Payload.Outline = []summaryOutline{{Kind: "phan", Label: "Phần I", Heading: "Hidden"}, {Kind: "muc", Label: "Mục 1", Heading: "Hidden"}, {Kind: "dieu", Label: "Điều 1", Heading: strings.Repeat("ố", 300)}, {Kind: "dieu", Label: "Điều 2", Heading: strings.Repeat("ố", 301)}, {Kind: "dieu", Label: "Điều 3", Anchor: "dieu-3", Heading: "phải làm"}}
	r, _, err := makeUnitSummary(d, []InventoryGap{{ItemKey: d.Key, Kind: "missing_effective_date"}}, map[string]bool{"dieu-3": true})
	if err != nil {
		t.Fatal(err)
	}
	for i, n := range r.Payload.Outline {
		if (n.Heading != "") != (i == 2) {
			t.Fatalf("outline %d: %+v", i, n)
		}
	}
}

func TestSummary_AllSiteStructureKindsSuppressHeadings(t *testing.T) {
	b, err := os.ReadFile("../../../../web/app/content/gaps.ts")
	if err != nil {
		t.Fatal(err)
	}
	list := regexp.MustCompile(`structure: \[([^\]]+)\]`).FindSubmatch(b)
	if len(list) != 2 {
		t.Fatal("cannot read authoritative structure category")
	}
	for _, match := range regexp.MustCompile(`'([^']+)'`).FindAllSubmatch(list[1], -1) {
		r, _, e := makeUnitSummary(summaryFixture(), []InventoryGap{{ItemKey: "vn:vbpl:1", Kind: string(match[1])}}, nil)
		if e != nil || r.Payload.Outline[0].Heading != "" {
			t.Fatalf("site structure kind %s exposes a heading: %v", match[1], e)
		}
	}
}

func TestSummary_PointersAndGaps(t *testing.T) {
	d := summaryFixture()
	for range 51 {
		d.Payload.Relations = append(d.Payload.Relations, summaryRelation{Title: "target"})
	}
	for range 11 {
		d.Payload.Nodes = append(d.Payload.Nodes, summaryNode{Code: "code", Name: "name"})
	}
	gaps := []InventoryGap{{ItemKey: d.Key, Kind: "consolidated_status_unresolved"}, {ItemKey: d.Key, Kind: "consolidated_status_unordered"}, {ItemKey: d.Key, Kind: "consolidated_status_unresolved"}, {ItemKey: "other", Kind: "no_tree"}}
	r, _, err := makeUnitSummary(d, gaps, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Payload.Relations) != 50 || r.Payload.RelationsTotal != 51 || len(r.Payload.Nodes) != 10 {
		t.Fatalf("caps: %+v", r.Payload)
	}
	b, _ := json.Marshal(r.GapKinds)
	if string(b) != `["consolidated_status_unordered","consolidated_status_unresolved"]` {
		t.Fatalf("gaps: %s", b)
	}
}

func TestSummary_ChapterSequence(t *testing.T) {
	for _, tc := range []struct {
		name   string
		labels []string
		valid  bool
	}{
		{"roman", []string{"I", "II", "III"}, true},
		{"restart", []string{"I", "II", "I", "II"}, true},
		{"letter", []string{"Chương c"}, false},
		{"skip", []string{"I", "III"}, false},
		{"repeat", []string{"I", "II", "II"}, false},
		{"digits", []string{"1", "2"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := summaryFixture()
			d.Payload.Outline = nil
			for _, label := range tc.labels {
				d.Payload.Outline = append(d.Payload.Outline, summaryOutline{Kind: "chuong", Label: label, Heading: "Chapter"})
			}
			d.Payload.Outline = append(d.Payload.Outline, summaryOutline{Kind: "dieu", Heading: "Article"})
			r, _, err := makeUnitSummary(d, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			for _, n := range r.Payload.Outline {
				if (n.Heading != "") != tc.valid {
					t.Errorf("%s: heading %q, valid %v", n.Label, n.Heading, tc.valid)
				}
			}
		})
	}
}

func TestSummary_HeadingShape(t *testing.T) {
	cases := []struct {
		heading string
		valid   bool
	}{{"Title)", true}, {"Title 2", true}, {"Title", true}}
	for _, mark := range []string{".", ":", ";", "?", "!"} {
		cases = append(cases, struct {
			heading string
			valid   bool
		}{"Title" + mark, false})
	}
	for _, mark := range []string{".", ":"} {
		for _, space := range []string{" ", "\u00a0", "\u2003"} {
			cases = append(cases, struct {
				heading string
				valid   bool
			}{"Title" + mark + space + "Text", false})
		}
	}
	for _, mark := range []string{"-", "+", "•", "*", "–", "—"} {
		cases = append(cases, struct {
			heading string
			valid   bool
		}{mark + " Title", false})
	}
	for _, tc := range cases {
		t.Run(tc.heading, func(t *testing.T) {
			d := summaryFixture()
			d.Payload.Outline[1].Heading = "  " + tc.heading + "  "
			r, _, err := makeUnitSummary(d, nil, nil)
			if err != nil || (r.Payload.Outline[1].Heading != "") != tc.valid {
				t.Fatalf("heading %q: got %q, err %v", tc.heading, r.Payload.Outline[1].Heading, err)
			}
		})
	}
}

func TestSummary_HeadingWhitespace(t *testing.T) {
	for _, tc := range []struct {
		heading string
		valid   bool
	}{
		{"Title.\u00a0Text", false}, {"Title:\u3000Text", false},
		{"\u00a0Title.\u3000", false}, {"Title.\u00a0\u3000Text", false},
		{"Title.\u001cText", true}, {"Title.\u001dText", true},
		{"Title.\u001eText", true}, {"Title.\u001fText", true},
		{"Title.\u001c", true}, {"Title.\u001d", true},
		{"Title.\u001e", true}, {"Title.\u001f", true},
	} {
		t.Run(fmt.Sprintf("%q", tc.heading), func(t *testing.T) {
			d := summaryFixture()
			d.Payload.Outline[1].Heading = tc.heading
			got, _, err := makeUnitSummary(d, nil, nil)
			if err != nil || (got.Payload.Outline[1].Heading != "") != tc.valid {
				t.Fatalf("heading=%q err=%v", got.Payload.Outline[1].Heading, err)
			}
		})
	}
}
