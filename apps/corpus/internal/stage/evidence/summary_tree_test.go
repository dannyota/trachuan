package evidence

import (
	"reflect"
	"strings"
	"testing"
)

func ptr[T any](v T) *T { return &v }

func treeRows(rows ...summaryProvision) []summaryProvision { return rows }

func row(id int64, parent int64, anchor string, assignments ...assignment) summaryProvision {
	p := summaryProvision{ID: id, Anchor: anchor, TextSHA256: hashPrefix + strings.Repeat("0", 64), Assignments: assignments}
	if parent != 0 {
		p.ParentID = &parent
	}
	return p
}

func lineFor(t *testing.T, lines []provisionLine, anchor string) provisionLine {
	t.Helper()
	for _, l := range lines {
		if l.Anchor == anchor {
			return l
		}
	}
	t.Fatalf("no line for %s", anchor)
	return provisionLine{}
}

func TestProvisionLines_ParentsAndAmbiguity(t *testing.T) {
	rows := treeRows(
		row(1, 0, "chuong-1"),
		row(2, 0, "chuong-1~2"),
		row(3, 1, "dieu-5"),
		row(4, 0, "chuong-2"),
		row(5, 4, "dieu-6"),
		row(6, 5, "dieu-6/khoan-1"),
		row(7, 5, "dieu-6/khoan-1~2"),
		row(8, 6, "dieu-6/khoan-1/diem-a"),
		row(9, 5, "dieu-6/khoan-2"),
	)
	sortRowsByAnchor(rows)
	lines, err := provisionLines("vn:vbpl:1", rows, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]struct {
		parent    *string
		ambiguous bool
	}{
		"chuong-1":              {nil, true},
		"chuong-1~2":            {nil, true},
		"dieu-5":                {ptr("chuong-1"), true},
		"chuong-2":              {nil, false},
		"dieu-6":                {ptr("chuong-2"), false},
		"dieu-6/khoan-1":        {ptr("dieu-6"), true},
		"dieu-6/khoan-1~2":      {ptr("dieu-6"), true},
		"dieu-6/khoan-1/diem-a": {ptr("dieu-6/khoan-1"), true},
		"dieu-6/khoan-2":        {ptr("dieu-6"), false},
	}
	for anchor, w := range want {
		got := lineFor(t, lines, anchor)
		if !reflect.DeepEqual(got.Parent, w.parent) || got.Ambiguous != w.ambiguous || got.Key != "vn:vbpl:1" || got.Type != recordProvision {
			t.Errorf("%s: got parent=%v ambiguous=%v, want %v %v", anchor, deref(got.Parent), got.Ambiguous, deref(w.parent), w.ambiguous)
		}
	}
	for i := 1; i < len(lines); i++ {
		if lines[i-1].Anchor >= lines[i].Anchor {
			t.Fatalf("lines not in anchor order: %s, %s", lines[i-1].Anchor, lines[i].Anchor)
		}
	}
}

func deref(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}

func TestProvisionLines_SuffixNeedsDigits(t *testing.T) {
	rows := treeRows(row(1, 0, "dieu-5~"), row(2, 0, "dieu-5~a"), row(3, 0, "dieu-6"))
	sortRowsByAnchor(rows)
	lines, err := provisionLines("vn:vbpl:1", rows, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range lines {
		if l.Ambiguous {
			t.Errorf("%s marked ambiguous", l.Anchor)
		}
	}
}

func TestProvisionLines_RefusesBrokenTrees(t *testing.T) {
	for name, rows := range map[string][]summaryProvision{
		"missing parent":   {row(1, 9, "dieu-1")},
		"duplicate anchor": {row(1, 0, "dieu-1"), row(2, 0, "dieu-1")},
		"out of order":     {row(1, 0, "dieu-2"), row(2, 0, "dieu-1")},
		"cycle":            {row(1, 2, "dieu-1"), row(2, 1, "dieu-2")},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := provisionLines("vn:vbpl:1", rows, nil); err == nil {
				t.Fatal("accepted")
			}
		})
	}
}

func TestProvisionLines_EffectiveNodesFollowServingRules(t *testing.T) {
	nodes := map[int64]taxonomyNode{
		11: {Code: "1.1.a", Served: true},
		12: {Code: "1.1.b", Served: true},
		13: {Code: "1.1.retired", Served: false},
		21: {Code: "2.1.a", Served: true},
		22: {Code: "2.1.b", Served: true},
		31: {Code: "3.1.a", Served: true},
	}
	seed := func(node, family int64) assignment {
		return assignment{Node: node, Family: family, Method: "phapdien_seed"}
	}
	manual := func(node, family int64, exclude bool) assignment {
		return assignment{Node: node, Family: family, Method: "manual", Exclude: exclude}
	}
	inherited := func(node, family int64) assignment {
		return assignment{Node: node, Family: family, Method: "inherited"}
	}
	llm := func(node, family int64) assignment { return assignment{Node: node, Family: family, Method: "llm"} }
	rows := treeRows(
		row(1, 0, "dieu-1", seed(11, 1), seed(13, 1), seed(21, 2), inherited(22, 2), llm(31, 3)),
		row(2, 1, "dieu-1/khoan-1"),
		row(3, 1, "dieu-1/khoan-2", manual(12, 1, false)),
		row(4, 1, "dieu-1/khoan-3", manual(21, 2, true)),
		row(5, 4, "dieu-1/khoan-3/diem-a"),
		row(6, 0, "dieu-2", inherited(22, 2), llm(21, 2)),
	)
	sortRowsByAnchor(rows)
	lines, err := provisionLines("vn:vbpl:1", rows, nodes)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{
		"dieu-1":                {"1.1.a", "2.1.a", "3.1.a"},
		"dieu-1/khoan-1":        {"1.1.a", "2.1.a", "3.1.a"},
		"dieu-1/khoan-2":        {"1.1.b", "2.1.a", "3.1.a"},
		"dieu-1/khoan-3":        {"1.1.a", "3.1.a"},
		"dieu-1/khoan-3/diem-a": {"1.1.a", "3.1.a"},
		"dieu-2":                {"2.1.b"},
	}
	for anchor, codes := range want {
		if got := lineFor(t, lines, anchor).Nodes; !reflect.DeepEqual(got, codes) {
			t.Errorf("%s: nodes %v, want %v", anchor, got, codes)
		}
	}
}

func TestProvisionLines_UnknownNodeRefused(t *testing.T) {
	rows := treeRows(row(1, 0, "dieu-1", assignment{Node: 99, Family: 1, Method: "manual"}))
	if _, err := provisionLines("vn:vbpl:1", rows, map[int64]taxonomyNode{}); err == nil {
		t.Fatal("accepted")
	}
}

func TestProvisionLines_ValidityRowsSortedAndEmptyListsKept(t *testing.T) {
	first := validityRow{StatusClass: "expired", EffFrom: ptr("2099-01-01"), EffTo: ptr("2099-02-01"), Method: "text"}
	second := validityRow{StatusClass: "expired", EffFrom: nil, EffTo: ptr("2099-03-01"), Method: "text"}
	third := validityRow{StatusClass: "in_force", EffFrom: ptr("2099-01-01"), Method: "manual"}
	p := row(1, 0, "dieu-1")
	p.Validity = []validityRow{third, first, second}
	lines, err := provisionLines("vn:vbpl:1", []summaryProvision{p, row(2, 0, "dieu-2")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := lines[0].Validity; !reflect.DeepEqual(got, []validityRow{second, first, third}) {
		t.Fatalf("validity %+v", got)
	}
	if lines[1].Validity == nil || len(lines[1].Validity) != 0 || lines[1].Nodes == nil || len(lines[1].Nodes) != 0 {
		t.Fatalf("empty lists %+v", lines[1])
	}
}

func TestRelationIdentity_EndsFollowMCPMeaning(t *testing.T) {
	key := func(s string) *string { return &s }
	base := summaryRelation{Kind: "repeals", SourceKey: "vn:vbpl:1", SourceNumber: "1/2099/TT", SourceInRelease: true, TargetKey: key("vn:vbpl:2"), TargetNumber: "2/2099/TT", TargetInRelease: true}
	for _, tc := range []struct {
		name   string
		method string
		anchor *string
		node   *string
		want   relationEnd
	}{
		{"text exact", "text_rule/1", ptr("dieu-8/khoan-9"), ptr("dieu-8/khoan-9"), relationEnd{Anchor: ptr("dieu-8/khoan-9"), Exact: ptr(true)}},
		{"text inexact", "text_rule/1", ptr("dieu-8"), ptr("dieu-8/khoan-9"), relationEnd{Anchor: ptr("dieu-8"), Exact: ptr(false), Named: ptr("dieu-8/khoan-9")}},
		{"text unresolved", "text_rule/1", nil, ptr("dieu-8/khoan-9"), relationEnd{Named: ptr("dieu-8/khoan-9")}},
		{"reviewed exact", "reviewed", ptr("dieu-3"), ptr("dieu-3"), relationEnd{Anchor: ptr("dieu-3"), Exact: ptr(true)}},
		{"reference resolved", "vbpl_references,vbpl_diagram", ptr("dieu-3"), ptr("node-77"), relationEnd{Anchor: ptr("dieu-3"), Exact: ptr(true)}},
		{"reference with text", "text_rule/1,vbpl_references", ptr("dieu-3"), ptr("node-77"), relationEnd{Anchor: ptr("dieu-3"), Exact: ptr(true)}},
		{"diagram unresolved node", "vbpl_diagram", nil, ptr("node-77"), relationEnd{Node: ptr("node-77")}},
		{"document level", "vbpl_diagram", nil, nil, relationEnd{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := base
			r.Method, r.TargetAnchor, r.TargetNode = tc.method, tc.anchor, tc.node
			got := relationFor(r)
			want := tc.want
			want.Key, want.Number, want.InRelease = key("vn:vbpl:2"), "2/2099/TT", true
			if !reflect.DeepEqual(got.Target, want) {
				t.Fatalf("target %+v, want %+v", got.Target, want)
			}
			if got.Source.Exact != nil || got.Source.Named != nil || got.Source.Node != nil || *got.Source.Key != "vn:vbpl:1" {
				t.Fatalf("source %+v", got.Source)
			}
		})
	}
}

func TestRelationLines_SortAndCountEqualRows(t *testing.T) {
	a := summaryRelation{Kind: "cites", Method: "vbpl_diagram", SourceKey: "vn:vbpl:2", SourceNumber: "2", TargetNumber: "x"}
	b := summaryRelation{Kind: "amends_supplements", Method: "vbpl_diagram", SourceKey: "vn:vbpl:1", SourceNumber: "1", TargetNumber: "y"}
	lines, err := relationLines([]summaryRelation{a, b, a, a})
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 2 || lines[0].line.Kind != "amends_supplements" || lines[0].line.Count != 1 || lines[1].line.Count != 3 {
		t.Fatalf("lines %+v", lines)
	}
}
