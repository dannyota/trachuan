package textparse

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestParse_ClausePointAcceptance(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		children   []string
		reason     string
	}{
		{"clauses with points", "1. Một\na) A\nb) B\n2. Hai", []string{"khoan-1|dieu-1", "khoan-1/diem-a|dieu-1/khoan-1", "khoan-1/diem-b|dieu-1/khoan-1", "khoan-2|dieu-1"}, ""},
		{"inserted", "1. Một\n2. Hai\n2a. Thêm\na) A\na1) A1\na2) A2\nb) B\nc) C\nd) D\nđ) Đ\n3. Ba", []string{"khoan-1|dieu-1", "khoan-2|dieu-1", "khoan-2a|dieu-1", "khoan-2a/diem-a|dieu-1/khoan-2a", "khoan-2a/diem-a1|dieu-1/khoan-2a", "khoan-2a/diem-a2|dieu-1/khoan-2a", "khoan-2a/diem-b|dieu-1/khoan-2a", "khoan-2a/diem-c|dieu-1/khoan-2a", "khoan-2a/diem-d|dieu-1/khoan-2a", "khoan-2a/diem-đ|dieu-1/khoan-2a", "khoan-3|dieu-1"}, ""},
		{"dash", "1- Một\na- A\nb- B\n2- Hai", []string{"khoan-1|dieu-1", "khoan-1/diem-a|dieu-1/khoan-1", "khoan-1/diem-b|dieu-1/khoan-1", "khoan-2|dieu-1"}, ""},
		{"footnote uppercase and bare", "1.[12] Một\n2.Hai\n3.", []string{"khoan-1|dieu-1", "khoan-2|dieu-1", "khoan-3|dieu-1"}, ""},
		{"sub items", "1. Một\na. A\nb. B\n- nhỏ\n+ nhỏ\n• nhỏ\n– nhỏ\n2. Hai", []string{"khoan-1|dieu-1", "khoan-1/diem-a|dieu-1/khoan-1", "khoan-1/diem-b|dieu-1/khoan-1", "khoan-2|dieu-1"}, ""},
		{"direct points", "a) A\nb) B", []string{"diem-a|dieu-1", "diem-b|dieu-1"}, ""},
		{"single clause", "1. Một", nil, "single_clause"},
		{"quoted amendments", "1. Thay bằng “\n2a) trích dẫn\nc.Nội dung\n”\n2. Hai", []string{"khoan-1|dieu-1", "khoan-2|dieu-1"}, ""},
		{"point tail", "1. Một\na) A\nb) B\nNội dung chung\n2. Hai", []string{"khoan-1|dieu-1", "khoan-2|dieu-1"}, "point_tail"},
		{"quoted duplicate", "1. Một\n2. Trích dẫn “\n1. Một\n”", nil, "containment"},
		{"clause gap", "1. Một\n3. Ba", nil, "clause_sequence"},
		{"clause repeat", "1. Một\n1. Lặp", nil, "clause_sequence"},
		{"clause style", "1. Một\n2- Hai", nil, "clause_style"},
		{"last clause candidate", "1. Một\n2. Hai\n2a) Sai", nil, "clause_candidate"},
		{"sub numbering", "1. Một\n2. Hai\n2.1. Sai", nil, "clause_candidate"},
		{"point gap", "a) A\nc) C", nil, "point_sequence"},
		{"single point", "a) A", nil, "point_sequence"},
		{"point style", "a) A\nb. B", nil, "point_style"},
		{"last point candidate", "a) A\nb) B\nc.Nội dung", nil, "point_candidate"},
		{"uppercase candidate", "a) A\nb) B\nC) Sai", nil, "point_candidate"},
		{"points before clauses", "a) A\n1. Một\n2. Hai", nil, "points_before_clauses"},
		{"inserted clause gap", "1. Một\n1b. Sai\n2. Hai", nil, "clause_sequence"},
		{"inserted point gap", "a) A\na2) Sai\nb) B", nil, "point_sequence"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Put the padding before the article so it cannot become a point tail.
			ps := paragraphs(strings.Repeat("Lời mở đầu ", 40) + "\nĐiều 1. Nội dung\n" + tc.body)
			got := Parse(ps)
			var children []string
			encoded, err := json.Marshal(got)
			if err != nil {
				t.Fatal(err)
			}
			var record struct {
				Provisions []struct{ Kind, Anchor, ParentAnchor string }
				Counts     struct {
					Structure *struct {
						ArticlesSplit, Clauses, Points int
						Reasons                        map[string]int
					}
				}
			}
			if err = json.Unmarshal(encoded, &record); err != nil {
				t.Fatal(err)
			}
			for _, p := range record.Provisions {
				if p.Kind == "khoan" || p.Kind == "diem" {
					children = append(children, strings.TrimPrefix(p.Anchor, "dieu-1/")+"|"+p.ParentAnchor)
				}
			}
			if !reflect.DeepEqual(children, tc.children) {
				t.Errorf("children=%v want %v", children, tc.children)
			}
			if record.Counts.Structure == nil {
				t.Fatal("missing structure counts")
			}
			if tc.reason != "" && record.Counts.Structure.Reasons[tc.reason] != 1 {
				t.Errorf("reason %s: %+v", tc.reason, record.Counts.Structure)
			}
			if len(got.Gaps) != 0 {
				t.Errorf("structure rejection must not create a quality gap: %v", got.Gaps)
			}
		})
	}
}

func TestParse_InstrumentChildren(t *testing.T) {
	got := Parse(paragraphs(strings.Repeat("Lời mở đầu ", 40) + "\nĐiều 1. Chính\nQUY CHẾ\nĐiều 1. Kèm theo\n1. Một\n2. Hai"))
	var anchors []string
	for _, p := range got.Provisions {
		if p.Kind == "khoan" {
			anchors = append(anchors, p.Anchor)
		}
	}
	if !reflect.DeepEqual(anchors, []string{"kem-theo-1/dieu-1/khoan-1", "kem-theo-1/dieu-1/khoan-2"}) {
		t.Fatal(anchors)
	}
}

func TestParse_ChildSpansAndIdentities(t *testing.T) {
	ps := paragraphs(strings.Repeat("Lời mở đầu ", 40) + "\nĐiều 1. Nội dung\nDẫn nhập\n1. Một\na) Điểm a\n- nhỏ\nb) Điểm b\n2. Hai\nĐoạn cuối của khoản")
	got := Parse(ps)
	old := ParseArticles(ps)
	counts := got.Counts
	counts.Structure = nil
	if counts != old.Counts {
		t.Fatal("input coverage changed")
	}
	if got.Counts.Structure.ArticlesSplit != 1 || got.Counts.Structure.Clauses != 2 || got.Counts.Structure.Points != 2 {
		t.Fatalf("structure counts=%+v", got.Counts.Structure)
	}
	if !reflect.DeepEqual(got.Scopes, old.Scopes) || !reflect.DeepEqual(got.Gaps, old.Gaps) {
		t.Fatal("scope checks changed")
	}
	parent := map[string]Provision{}
	for _, p := range got.Provisions {
		parent[p.Anchor] = p
	}
	for _, p := range old.Provisions {
		if parent[p.Anchor] != p {
			t.Fatalf("article or structure changed: %+v != %+v", parent[p.Anchor], p)
		}
	}
	for _, want := range []struct {
		anchor, text, label, parent string
		ordinal, start, end         int
	}{
		{"dieu-1/khoan-1", "1. Một\na) Điểm a\n- nhỏ\nb) Điểm b", "Khoản 1", "dieu-1", 1, 4, 7},
		{"dieu-1/khoan-1/diem-a", "a) Điểm a\n- nhỏ", "Điểm a", "dieu-1/khoan-1", 1, 5, 6},
		{"dieu-1/khoan-1/diem-b", "b) Điểm b", "Điểm b", "dieu-1/khoan-1", 2, 7, 7},
		{"dieu-1/khoan-2", "2. Hai\nĐoạn cuối của khoản", "Khoản 2", "dieu-1", 2, 8, 9},
	} {
		p := parent[want.anchor]
		if p.Text != want.text || p.Label != want.label || p.ParentAnchor != want.parent || p.Ordinal != want.ordinal || p.StartOrdinal != want.start || p.EndOrdinal != want.end || p.Heading != "" {
			t.Errorf("%s: %+v", want.anchor, p)
		}
		if strings.Count(parent[want.parent].Text, p.Text) != 1 {
			t.Error("nonunique containment")
		}
		sum := sha256.Sum256([]byte(fmt.Sprintf("%d\n%s\n%s", p.ScopeIndex, p.Anchor, p.Text)))
		if p.NodeID != fmt.Sprintf("tx:%x", sum[:8]) {
			t.Errorf("node id=%s", p.NodeID)
		}
	}
	changed := append([]Paragraph(nil), ps...)
	changed[4].Text += " mới"
	next := Parse(changed)
	for _, p := range next.Provisions {
		if p.Anchor == "dieu-1/khoan-1/diem-a" && p.NodeID == parent[p.Anchor].NodeID {
			t.Fatal("changed child kept id")
		}
	}
}

func TestParse_ScopeBoundariesExcludeChildren(t *testing.T) {
	for _, text := range []string{
		"Điều 1. Một\nĐiều 3. Ba\n1. Khoản\n2. Khoản",
		"Điều 1. Một\nPHỤ LỤC\n1. Khoản\n2. Khoản",
		"Chương I\n1. Khoản\n2. Khoản\nĐiều 1. Một",
	} {
		got := Parse(paragraphs(strings.Repeat("Lời mở đầu ", 40) + "\n" + text))
		for _, p := range got.Provisions {
			if p.Kind == "khoan" || p.Kind == "diem" {
				t.Fatalf("children outside accepted article: %+v", p)
			}
		}
	}
}

func TestParse_PointContainmentAndTail(t *testing.T) {
	for _, tc := range []struct {
		body, reason string
		points       int
	}{
		{"1. Dẫn “\na) Trùng\n”\na) Trùng\nb) Cuối\n2. Hai", "containment", 0},
		{"1. Một\na) A\n2. Hai", "", 1},
		{"a) A\nb) B\nĐoạn chung", "point_tail", 0},
	} {
		got := Parse(paragraphs(strings.Repeat("Lời mở đầu ", 40) + "\nĐiều 1. Nội dung\n" + tc.body))
		if got.Counts.Structure.Points != tc.points || (tc.reason != "" && got.Counts.Structure.Reasons[tc.reason] != 1) {
			t.Fatalf("body=%s counts=%+v", tc.body, got.Counts.Structure)
		}
	}
}

func TestParse_ParentAnchorsIncludeStructure(t *testing.T) {
	got := Parse(paragraphs(strings.Repeat("Lời mở đầu ", 40) + "\nPhần I\nChương I\nMục 1\nĐiều 1. Một\n1. Khoản một\n2. Khoản hai\nQUY CHẾ\nĐiều 1. Kèm theo\na) A\nb) B"))
	var parents []string
	for _, p := range got.Provisions {
		if p.ScopeIndex >= 0 {
			parents = append(parents, p.Anchor+"|"+p.ParentAnchor)
		}
	}
	want := []string{"phan-i|", "chuong-i|phan-i", "muc-1|chuong-i", "dieu-1|muc-1", "dieu-1/khoan-1|dieu-1", "dieu-1/khoan-2|dieu-1", "doan-2|", "kem-theo-1/dieu-1|", "kem-theo-1/dieu-1/diem-a|kem-theo-1/dieu-1", "kem-theo-1/dieu-1/diem-b|kem-theo-1/dieu-1"}
	if !reflect.DeepEqual(parents, want) {
		t.Fatalf("parents=%v want %v", parents, want)
	}
}
