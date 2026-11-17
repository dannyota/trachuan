package build

import (
	"context"
	"encoding/json"
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

func TestSummary_Caps(t *testing.T) {
	for _, field := range []string{"number", "issuer", "type", "title", "relation_title", "topic_name"} {
		t.Run(field, func(t *testing.T) {
			d := summaryFixture()
			switch field {
			case "number":
				d.Payload.Number = strings.Repeat("ố", 201)
			case "issuer":
				d.Payload.Issuer = strings.Repeat("ố", 201)
			case "type":
				d.Payload.Type = strings.Repeat("ố", 201)
			case "title":
				d.Payload.Title = strings.Repeat("ố", 1001)
			case "relation_title":
				d.Payload.Relations = []summaryRelation{{Title: strings.Repeat("ố", 1001)}}
			case "topic_name":
				d.Payload.Nodes = []summaryNode{{Name: strings.Repeat("ố", 201)}}
			}
			if _, _, err := makeUnitSummary(d, nil, nil); err == nil {
				t.Fatal("accepted over-cap " + field)
			}
		})
	}
	d := summaryFixture()
	d.Payload.Number, d.Payload.Issuer, d.Payload.Type = strings.Repeat("ố", 200), strings.Repeat("ố", 200), strings.Repeat("ố", 200)
	d.Payload.Title = strings.Repeat("ố", 1000)
	d.Payload.Relations = []summaryRelation{{Title: strings.Repeat("ố", 1000)}}
	d.Payload.Nodes = []summaryNode{{Name: strings.Repeat("ố", 200)}}
	if _, _, err := makeUnitSummary(d, nil, nil); err != nil {
		t.Fatal(err)
	}
}

func TestSummary_EligibilityAndOutlineCap(t *testing.T) {
	for _, tc := range []struct {
		class           string
		count           int
		label           string
		indexed, capped bool
	}{
		{"primary", 1, "Điều 1", true, false}, {"relation_context", 1, "Điều 1", false, false}, {"secondary", 1, "Điều 1", false, false},
		{"primary", 0, "", false, false}, {"primary", 1, "", false, false}, {"primary", 1500, "x", true, false}, {"primary", 1501, "x", false, true},
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
