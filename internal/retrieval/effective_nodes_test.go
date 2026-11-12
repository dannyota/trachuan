package retrieval

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

func TestHits_NodesDeepestFirst(t *testing.T) {
	a, r := shapeFixture("Nội dung")
	a.EffectiveNodes = map[int64][]string{2: {"1.1.c1", "2.1.c1", "3.1"}}
	a.Nodes = map[string]NodeRef{"1.1.c1": {Name: "One", Level: 3}, "2.1.c1": {Name: "Two", Level: 3}, "3.1": {Name: "Three", Level: 2}}
	out, err := Shape(a, r, 8, "dense")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(out.Hits[0].Nodes, a.EffectiveNodes[2]) {
		t.Fatal("hit lost effective nodes", out.Hits[0].Nodes)
	}
	if !reflect.DeepEqual(out.Nodes, a.Nodes) {
		t.Fatal("index missing nodes", out.Nodes)
	}
}

func TestFilter_UnservedCodeInvalid(t *testing.T) {
	for _, refs := range []map[string]NodeRef{nil, {"1": {Name: "Root", Level: 1}}} {
		if err := validateServedFilters(Request{Nodes: []string{"1.1"}}, refs); !errors.Is(err, ErrInvalidRequest) {
			t.Fatal(err)
		}
	}
	if err := validateServedFilters(Request{Nodes: []string{"1"}}, map[string]NodeRef{"1": {Level: 1}}); err != nil {
		t.Fatal(err)
	}
}

func TestNodes_MetadataCache(t *testing.T) {
	doc := DocumentEntry{Nodes: []string{"1.1.c1", "2.1"}, Status: Status{Current: true}}
	raw := metadataJSON(doc)
	var got DocumentEntry
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Nodes, doc.Nodes) || got.Status.Current {
		t.Fatal("cached nodes or status", got)
	}
}

func TestNodes_Presentation(t *testing.T) {
	a, r := shapeFixture("Nội dung")
	out, err := Shape(a, r, 8, "dense")
	if err != nil {
		t.Fatal(err)
	}
	yes := true
	out.Hits[0].Nodes = []string{"1.1"}
	out.Hits[0].InFilter = &yes
	out.Nodes = map[string]NodeRef{"1.1": {Name: "Family", Level: 2}, "2.1": {Name: "Other", Level: 2}}
	out.NextCursor = "next"
	n := 3
	out.OutsideFilterCount = &n
	for _, detail := range []Detail{DetailCompact, DetailFull} {
		result := present(out, map[string]DocumentEntry{"vn:1": {Nodes: []string{"2.1"}}}, detail)
		raw, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		var got map[string]any
		if err = json.Unmarshal(raw, &got); err != nil {
			t.Fatal(err)
		}
		if got["next_cursor"] != "next" || got["outside_filter_count"] != float64(3) || got["nodes"] == nil {
			t.Fatal("lost paging/index fields", string(raw))
		}
		hit := got["hits"].([]any)[0].(map[string]any)
		if hit["in_filter"] != true || hit["nodes"] == nil {
			t.Fatal("lost hit fields", hit)
		}
	}
}
