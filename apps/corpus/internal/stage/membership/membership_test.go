package membership

import (
	"bytes"
	"strings"
	"testing"
)

func TestMembership_TypedSupportTuples(t *testing.T) {
	a := SupportTuple{RuleKind: "term", RuleValue: "law", Evidence: EvidenceRef{ResponseSHA256: "abc", ArtifactHash: "def", Path: "response.json"}}
	b := SupportTuple{RuleKind: "agency", RuleValue: "1", Evidence: EvidenceRef{ResponseSHA256: "123", ArtifactHash: "456", Path: "listing.json"}}
	x := []Entry{{DocumentKey: "vn:vbpl:2", Supports: []SupportTuple{a, b}}, {DocumentKey: "vn:vbpl:1", Supports: []SupportTuple{b, a}}}
	y := []Entry{{DocumentKey: "vn:vbpl:1", Supports: []SupportTuple{a, b}}, {DocumentKey: "vn:vbpl:2", Supports: []SupportTuple{b, a}}}
	xb, xh, err := CanonicalSnapshot(x)
	if err != nil {
		t.Fatal(err)
	}
	yb, yh, err := CanonicalSnapshot(y)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(xb, yb) || xh != yh || len(xh) != 64 {
		t.Fatalf("snapshot differs: %s %s", xb, yb)
	}
}

func TestMembership_ClassPrecedenceAndSeeds(t *testing.T) {
	for _, tc := range []struct{ a, b, want string }{{"relation_context", "primary", "primary"}, {"primary", "relation_context", "primary"}, {"secondary", "secondary", "secondary"}} {
		got, err := MergeClass(tc.a, tc.b)
		if err != nil || got != tc.want {
			t.Fatalf("MergeClass(%s,%s) = %s, %v", tc.a, tc.b, got, err)
		}
	}
	if _, err := MergeClass("primary", "secondary"); err == nil {
		t.Fatal("merged separate secondary class")
	}
	a := SupportTuple{RuleKind: "agency", RuleValue: "1"}
	b := SupportTuple{RuleKind: "term", RuleValue: "law"}
	if got := MergeSupports([]SupportTuple{a}, []SupportTuple{a, b}); len(got) != 2 {
		t.Fatalf("supports = %#v", got)
	}
}

func TestMembership_LeavesOnlyWithoutSupport(t *testing.T) {
	for _, tc := range []struct {
		current, retained int
		want              bool
	}{{0, 0, true}, {1, 0, false}, {0, 1, false}, {1, 1, false}} {
		if got := LeavesScope(tc.current, tc.retained); got != tc.want {
			t.Fatalf("LeavesScope(%d,%d) = %t", tc.current, tc.retained, got)
		}
	}
}

func TestSnapshot_ExplicitNullsAndEmptyArrays(t *testing.T) {
	b, _, err := CanonicalSnapshot([]Entry{{DocumentKey: "vn:vbpl:1"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{`"raw_number":null`, `"normalized_number":null`, `"replacement_key":null`, `"supports":[]`} {
		if !strings.Contains(string(b), s) {
			t.Errorf("missing %s in %s", s, b)
		}
	}
}
