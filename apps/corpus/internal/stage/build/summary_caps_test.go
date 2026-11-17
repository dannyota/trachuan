package build

import (
	"context"
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
		sizes   []int
		fail    bool
	}{
		{name: "whole", count: 2, label: "Điều 1", heading: "Scope", step: "whole", sizes: []int{65536}},
		{name: "headings_dropped", count: 200, label: "Điều 1", heading: strings.Repeat("ố", 100), step: "headings_dropped", sizes: []int{65537, 65536}},
		{name: "outline_dropped_by_bytes", count: 200, label: strings.Repeat("ố", 120), heading: "Scope", step: "outline_dropped", sizes: []int{65537, 65537, 65536}},
		{name: "outline_dropped_by_entries", count: 1501, label: "x", heading: "Scope", step: "outline_dropped", sizes: []int{65536, 65536, 65536}},
		{name: "outline_dropped_by_both", count: 1501, label: "x", heading: strings.Repeat("ố", 100), step: "outline_dropped", sizes: []int{65537, 65537, 65536}},
		{name: "without_outline_still_over", count: 200, label: "Điều 1", heading: "Scope", url: strings.Repeat("a", 65536), step: "outline_dropped", sizes: []int{65537, 65537, 65537}, fail: true},
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
					r, step, err := makeSummary(context.Background(), &summaryTx{sizes: tc.sizes}, d, nil, nil)
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
