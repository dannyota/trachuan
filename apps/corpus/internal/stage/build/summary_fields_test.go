package build

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestLoad_SummaryFieldBounds(t *testing.T) {
	for _, tc := range []struct {
		field string
		cap   int
	}{
		{"number", 200}, {"type", 200}, {"issuer", 200}, {"title", 1000},
		{"relation.number", 200}, {"relation.title", 1000}, {"topic.name", 600},
	} {
		for _, over := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/over=%t", tc.field, over), func(t *testing.T) {
				d := summaryFixture()
				count := tc.cap
				if over {
					count++
				}
				value := strings.Repeat("ố", count)
				switch tc.field {
				case "number":
					d.Payload.Number = value
				case "type":
					d.Payload.Type = value
				case "issuer":
					d.Payload.Issuer = value
				case "title":
					d.Payload.Title = value
				case "relation.number":
					d.Payload.Relations = []summaryRelation{{Number: value, Title: "target"}}
				case "relation.title":
					d.Payload.Relations = []summaryRelation{{Number: "1", Title: value}}
				case "topic.name":
					d.Payload.Nodes = []summaryNode{{Code: "topic", Name: value}}
				}
				tx := &summaryTx{documents: []summaryDocument{d}, sizes: []int{65536}}
				l := loader{tx: tx, docs: map[string]*loadedDocument{d.Key: {id: 1}}}
				if err := l.loadSummaries(context.Background(), nil); err != nil {
					t.Fatal(err)
				}
				wantGaps := 0
				if over {
					wantGaps = 1
				}
				if len(l.gaps) != wantGaps {
					t.Fatalf("gaps=%+v, want %d", l.gaps, wantGaps)
				}
				for _, gap := range l.gaps {
					if gap.Stage != "build" || gap.ItemKey != d.Key || gap.Kind != "summary_field_capped" || gap.Detail != tc.field {
						t.Fatalf("gap=%+v", gap)
					}
				}
				preview := strings.Contains(tc.field, ".")
				if over && !preview {
					if len(tx.inserts) != 0 || tx.sizeCalls != 0 {
						t.Fatalf("over-cap document inserted or measured: %d/%d", len(tx.inserts), tx.sizeCalls)
					}
					return
				}
				if len(tx.inserts) != 1 {
					t.Fatalf("inserts=%d", len(tx.inserts))
				}
				var p summaryPayload
				if err := json.Unmarshal(tx.inserts[0][11].([]byte), &p); err != nil {
					t.Fatal(err)
				}
				got := map[string]string{"number": p.Number, "title": p.Title, "type": p.Type, "issuer": p.Issuer}[tc.field]
				if strings.HasPrefix(tc.field, "relation.") {
					if p.RelationsTotal != 1 || len(p.Relations) != 1-wantGaps {
						t.Fatalf("relations=%+v total=%d", p.Relations, p.RelationsTotal)
					}
					if !over {
						got = p.Relations[0].Number
						if tc.field == "relation.title" {
							got = p.Relations[0].Title
						}
					}
				} else if tc.field == "topic.name" {
					if len(p.Nodes) != 1-wantGaps {
						t.Fatalf("nodes=%+v", p.Nodes)
					}
					if !over {
						got = p.Nodes[0].Name
					}
				}
				if !over && got != value {
					t.Fatal("at-cap value changed")
				}
			})
		}
	}
}

func TestLoad_SummaryFiltersBeforePreviewCuts(t *testing.T) {
	d := summaryFixture()
	d.Payload.Relations = []summaryRelation{{Number: strings.Repeat("a", 201)}, {Title: strings.Repeat("a", 1001)}}
	d.Payload.Nodes = []summaryNode{{Name: strings.Repeat("a", 601)}}
	for i := range 51 {
		d.Payload.Relations = append(d.Payload.Relations, summaryRelation{Number: fmt.Sprint(i)})
	}
	for i := range 11 {
		d.Payload.Nodes = append(d.Payload.Nodes, summaryNode{Code: fmt.Sprint(i), Name: "name"})
	}
	tx := &summaryTx{documents: []summaryDocument{d, summaryFixture()}, sizes: []int{65536, 65536}}
	tx.documents[1].Key = "vn:vbpl:2"
	l := loader{tx: tx, docs: map[string]*loadedDocument{d.Key: {id: 1}, "vn:vbpl:2": {id: 2}}}
	if err := l.loadSummaries(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if len(tx.inserts) != 2 || len(l.gaps) != 3 {
		t.Fatalf("inserts=%d gaps=%+v", len(tx.inserts), l.gaps)
	}
	var p summaryPayload
	if err := json.Unmarshal(tx.inserts[0][11].([]byte), &p); err != nil {
		t.Fatal(err)
	}
	if p.RelationsTotal != 53 || len(p.Relations) != 50 || len(p.Nodes) != 10 {
		t.Fatalf("total=%d relations=%d nodes=%d", p.RelationsTotal, len(p.Relations), len(p.Nodes))
	}
	for i, r := range p.Relations {
		if r.Number != fmt.Sprint(i) {
			t.Fatalf("relation %d=%+v", i, r)
		}
	}
	for i, n := range p.Nodes {
		if n.Code != fmt.Sprint(i) {
			t.Fatalf("node %d=%+v", i, n)
		}
	}
}

func TestLoad_SummaryRowCappedContinues(t *testing.T) {
	tx := &summaryTx{documents: []summaryDocument{summaryFixture(), summaryFixture()}, sizes: []int{65537, 65537, 65537, 65536}}
	tx.documents[0].Payload.Outline = nil
	tx.documents[1].Key = "vn:vbpl:2"
	l := loader{tx: tx, docs: map[string]*loadedDocument{"vn:vbpl:1": {id: 1}, "vn:vbpl:2": {id: 2}}}
	if err := l.loadSummaries(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if len(tx.inserts) != 1 || tx.inserts[0][0] != "vn:vbpl:2" {
		t.Fatalf("inserts=%v", tx.inserts)
	}
	if len(l.gaps) != 2 || l.counts.Summary.OutlineDropped != 1 {
		t.Fatalf("gaps=%+v stats=%+v", l.gaps, l.counts.Summary)
	}
	for i, kind := range []string{"summary_outline_capped", "summary_row_capped"} {
		gap := l.gaps[i]
		if gap.Stage != "build" || gap.ItemKey != "vn:vbpl:1" || gap.Kind != kind || gap.Detail != "" {
			t.Errorf("gap=%+v, want %s", gap, kind)
		}
	}
	if err := l.loadGaps(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestSummary_CapGapsKeepHeadings(t *testing.T) {
	for _, kind := range []string{"summary_field_capped", "summary_row_capped"} {
		t.Run(kind, func(t *testing.T) {
			d, _, err := makeUnitSummary(summaryFixture(), []InventoryGap{{ItemKey: "vn:vbpl:1", Kind: kind}}, nil)
			if err != nil || len(d.Payload.Outline) != 2 || d.Payload.Outline[0].Heading != "General" {
				t.Fatalf("%s: outline=%+v error=%v", kind, d.Payload.Outline, err)
			}
		})
	}
}
