package build

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func summaryOutlineFixture(count int, label, heading string) summaryDocument {
	d := summaryFixture()
	d.Payload.Outline = make([]summaryOutline, count)
	for i := range d.Payload.Outline {
		d.Payload.Outline[i] = summaryOutline{Kind: "dieu", Label: label, Anchor: fmt.Sprint(i), Heading: heading}
	}
	return d
}

func TestSummary_OrderedCaps(t *testing.T) {
	for _, tc := range []struct {
		name    string
		count   int
		label   string
		heading string
		url     string
		step    string
		fail    bool
	}{
		{name: "whole", count: 2, label: "Điều 1", heading: "Scope", step: "whole"},
		{name: "headings_dropped", count: 200, label: "Điều 1", heading: strings.Repeat("ố", 100), step: "headings_dropped"},
		{name: "outline_dropped_by_bytes", count: 200, label: strings.Repeat("ố", 120), heading: "Scope", step: "outline_dropped"},
		{name: "outline_dropped_by_entries", count: 1501, label: "x", heading: "Scope", step: "outline_dropped"},
		{name: "outline_dropped_by_both", count: 1501, label: "x", heading: strings.Repeat("ố", 100), step: "outline_dropped"},
		{name: "without_outline_still_over", count: 200, label: "Điều 1", heading: "Scope", url: strings.Repeat("a", 65536), step: "outline_dropped", fail: true},
	} {
		for _, class := range []string{"primary", "relation_context", "secondary"} {
			for _, labeled := range []bool{true, false} {
				t.Run(fmt.Sprintf("%s/%s/labeled=%t", tc.name, class, labeled), func(t *testing.T) {
					label := tc.label
					if !labeled {
						label = strings.Repeat(" ", len(label))
					}
					d := summaryOutlineFixture(tc.count, label, tc.heading)
					d.Class, d.Indexed = class, true
					if tc.url != "" {
						d.Payload.SourceURL = new(tc.url)
					}
					before, err := json.Marshal(d)
					if err != nil {
						t.Fatal(err)
					}
					r, step, err := makeSummary(d, nil, nil)
					if tc.fail {
						want := "summary vn:vbpl:1: serialized row exceeds 65536 octets"
						if err == nil || err.Error() != want {
							t.Fatalf("error=%v, want %s", err, want)
						}
					} else {
						if err != nil {
							t.Fatal(err)
						}
						wantIndexed := class == "primary" && labeled && tc.step != "outline_dropped"
						if r.Indexed != wantIndexed {
							t.Errorf("indexed=%t, want %t", r.Indexed, wantIndexed)
						}
						wantOutline := append([]summaryOutline{}, d.Payload.Outline...)
						switch tc.step {
						case "headings_dropped":
							for i := range wantOutline {
								wantOutline[i].Heading = ""
							}
						case "outline_dropped":
							wantOutline = []summaryOutline{}
						}
						if !reflect.DeepEqual(r.Payload.Outline, wantOutline) {
							t.Errorf("outline differs from %s result", tc.step)
						}
						b, e := json.Marshal(r)
						if e != nil || len(b) > 65536 {
							t.Errorf("serialized bytes=%d: %v", len(b), e)
						}
					}
					if string(step) != tc.step {
						t.Errorf("step=%v, want %s", step, tc.step)
					}
					after, e := json.Marshal(d)
					if e != nil || string(before) != string(after) {
						t.Errorf("input changed: %v", e)
					}
				})
			}
		}
	}
}

func TestSummary_RowSizeBoundary(t *testing.T) {
	for _, size := range []int{65535, 65536, 65537} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			d := summaryFixture()
			d.Projection, d.Indexed = 1, true
			d.GapKinds = []string{}
			d.Payload.Relations = []summaryRelation{}
			d.Payload.Nodes = []summaryNode{}
			d.Payload.SourceURL = new("")
			b, err := json.Marshal(d)
			if err != nil {
				t.Fatal(err)
			}
			d.Payload.SourceURL = new(strings.Repeat("a", size-len(b)))
			r, step, err := makeSummary(d, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			wantStep := "whole"
			if size > 65536 {
				wantStep = "headings_dropped"
			}
			if string(step) != wantStep || !r.Indexed {
				t.Errorf("step=%v indexed=%t, want %s and indexed", step, r.Indexed, wantStep)
			}
			if size <= 65536 && !reflect.DeepEqual(r, d) {
				t.Error("row within both caps changed")
			}
			if size > 65536 && (r.Payload.Outline[0].Heading != "" || r.Payload.Outline[1].Heading != "") {
				t.Error("headings retained over byte cap")
			}
		})
	}
}
