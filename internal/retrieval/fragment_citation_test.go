package retrieval

import (
	"encoding/json"
	"testing"
)

func TestEvidence_FragmentCitationsAndSerialization(t *testing.T) {
	for _, sibling := range []bool{false, true} {
		t.Run(map[bool]string{false: "parent", true: "siblings"}[sibling], func(t *testing.T) {
			var a Arms
			var r Recipe
			if sibling {
				a, r, _ = siblingFixture(t)
				c := &a.Primary.Dense[0]
				c.Provision.Label = "Khoản 2"
				c.Ancestors[1].Label = "Khoản 2"
				c.Descendants[0].Label = "Khoản 2"
				point := Provision{ID: 8, ParentID: &c.GroupID, Identity: Identity{Country: "vn", DocumentKey: "vn:1", Anchor: "point"}, NodeID: "p", Kind: "diem", Label: "Điểm a", Text: "point"}
				c.Descendants = append(c.Descendants, point)
				a.Primary.Members[0] = *c
			} else {
				a, r = shapeFixture("clause")
				c := &a.Primary.Dense[0]
				point := Provision{ID: 3, ParentID: &c.GroupID, Identity: Identity{Country: "vn", DocumentKey: "vn:1", Anchor: "point"}, NodeID: "p", Kind: "diem", Label: "Điểm a", Text: "point"}
				c.Descendants = append(c.Descendants, point)
				a.Primary.Members[0] = *c
			}
			out, err := Shape(a, r, 8, "dense")
			if err != nil {
				t.Fatal(err)
			}
			want := map[string]string{"clause": "Điều 1, Khoản 1", "point": "Điều 1, Khoản 1, Điểm a"}
			if sibling {
				want = map[string]string{"article": "Điều 1", "clause-1": "Điều 1, Khoản 1", "clause-2": "Điều 1, Khoản 2", "point": "Điều 1, Khoản 2, Điểm a"}
			}
			for _, f := range out.Hits[0].Fragments {
				if f.Citation != want[f.Identity.Anchor] {
					t.Errorf("%s citation %q, want %q", f.Identity.Anchor, f.Citation, want[f.Identity.Anchor])
				}
				delete(want, f.Identity.Anchor)
			}
			if len(want) != 0 {
				t.Errorf("missing fragments: %v", want)
			}
			data, err := json.Marshal(out.Hits[0])
			if err != nil {
				t.Fatal(err)
			}
			var decoded struct {
				Fragments []map[string]any `json:"fragments"`
			}
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatal(err)
			}
			for _, f := range decoded.Fragments {
				if _, ok := f["citation"]; ok {
					t.Fatal("fragment citation serialized")
				}
			}
		})
	}
}
