package retrieval

import (
	"reflect"
	"strings"
	"testing"
	"unicode"
)

func TestSplitParts_LosslessAndPositive(t *testing.T) {
	for _, text := range []string{"", "a", strings.Repeat("điều kiện \n", 7000), strings.Repeat("á", 40000)} {
		parts := splitParts([]rune(text), PartSize)
		var joined []rune
		for _, p := range parts {
			if len(p) < 1 || len(p) > PartSize {
				t.Fatalf("part length %d", len(p))
			}
			joined = append(joined, p...)
		}
		if string(joined) != text {
			t.Fatal("parts lost source text")
		}
	}
}

func TestSplitParts_NoWhitespaceKeepsCombiningMark(t *testing.T) {
	text := strings.Repeat("a", PartSize-1) + "e\u0301" + strings.Repeat("b", 20)
	parts := splitParts([]rune(text), PartSize)
	if len(parts) != 2 || len(parts[0]) != PartSize-1 || unicode.Is(unicode.Mn, parts[1][0]) || string(parts[1][:2]) != "e\u0301" {
		t.Fatal("split separated a combining mark")
	}
}

func TestSplitParts_FixedBoundaries(t *testing.T) {
	r := []rune(strings.Repeat("văn bản pháp luật\n", 3000))
	a, b := splitParts(r, PartSize), splitParts(r, PartSize)
	if !reflect.DeepEqual(a, b) {
		t.Fatal("boundaries changed")
	}
	for _, p := range a[:len(a)-1] {
		if !unicode.IsSpace(p[len(p)-1]) {
			t.Fatal("did not cut at whitespace")
		}
	}
}

func TestReadParts_SerializedBudgetAndLossless(t *testing.T) {
	for _, u := range []Unit{
		{Kind: "provision", Handle: "vn:1#p1", Text: strings.Repeat("\"\\\n<&e\u0301 ", 6000)},
		{Kind: "outline", Handle: "vn:1#p1", Heading: strings.Repeat("đ", 40000), Label: "Điều 1"},
		{Kind: "relation", Relation: &ListedRelation{RawLabel: strings.Repeat("nhãn ", 10000), Evidence: "evidence", EvidenceComplete: true}},
	} {
		parts := unitParts(u)
		var text, heading, label, evidence, rawLabel strings.Builder
		for i, p := range parts {
			if readSize(p) > PartSize {
				t.Fatalf("serialized part %d exceeds budget: %d", i, readSize(p))
			}
			if p.Part != i+1 || p.Parts != len(parts) {
				t.Fatal("part numbering")
			}
			text.WriteString(p.Text)
			heading.WriteString(p.Heading)
			label.WriteString(p.Label)
			if p.Relation != nil {
				evidence.WriteString(p.Relation.Evidence)
				rawLabel.WriteString(p.Relation.RawLabel)
			}
		}
		if text.String() != u.Text {
			t.Fatal("lost text")
		}
		if u.Kind == "outline" && heading.String() != u.Heading {
			t.Fatal("lost heading")
		}
		if u.Relation != nil && (evidence.String() != u.Relation.Evidence || rawLabel.String() != u.Relation.RawLabel) {
			t.Fatal("lost relation text")
		}
	}
}

func TestReadLeads_ContainedAndAmbiguous(t *testing.T) {
	parent := int64(1)
	nodes := []Provision{
		{ID: 1, Identity: Identity{DocumentKey: "vn:1", Anchor: "a"}, Kind: "dieu", Label: "Điều 1", Text: "Condition:\nFirst duty.\nSecond duty."},
		{ID: 2, ParentID: &parent, Identity: Identity{DocumentKey: "vn:1", Anchor: "b"}, Kind: "khoan", Text: "First duty."},
		{ID: 3, ParentID: &parent, Identity: Identity{DocumentKey: "vn:1", Anchor: "c"}, Kind: "khoan", Text: "Second duty."},
	}
	leads, gaps := readLeads(nodes, nodes[1])
	if len(gaps) != 0 || len(leads) != 1 || leads[0].Text != "Condition:" {
		t.Fatalf("contained lead: %+v %+v", leads, gaps)
	}
	nodes[0].Text = "Condition:\nFirst duty."
	leads, gaps = readLeads(nodes, nodes[1])
	if len(leads) != 0 || len(gaps) != 1 || gaps[0].Kind != "ambiguous_tree_text" {
		t.Fatal("ambiguous lead was accepted")
	}
}

func TestReadOwnText_ContainedChildOnce(t *testing.T) {
	parent := int64(1)
	nodes := []Provision{
		{ID: 1, Identity: Identity{DocumentKey: "vn:1", Anchor: "a"}, Kind: "khoan", Text: "Condition:\nFirst duty.\nClosing."},
		{ID: 2, ParentID: &parent, Identity: Identity{DocumentKey: "vn:1", Anchor: "b"}, Kind: "diem", Text: "First duty."},
	}
	text, gaps := readOwnText(nodes, nodes[0])
	if text != "Condition:\n\nClosing." || len(gaps) != 0 {
		t.Fatalf("own text: %q %+v", text, gaps)
	}
	nodes[0].Text = "First duty. First duty."
	_, gaps = readOwnText(nodes, nodes[0])
	if len(gaps) != 1 || gaps[0].Kind != "ambiguous_tree_text" {
		t.Fatal("ambiguous own text was accepted")
	}
}

func TestReadPage_EmptyOutlineCountsDocument(t *testing.T) {
	seq := readSequence{document: &DocumentEntry{Title: strings.Repeat("x", 100)}, complete: true}
	c := cursor{V: 1, Release: "release", Item: ReadItem{Handle: "vn:1"}}
	deferred, cost, err := readPage(seq, c, 1, false)
	if err != nil || deferred.NotRead != "call_cap" || deferred.Next == "" || deferred.Document != nil || cost != 0 {
		t.Fatal("document bypassed call cap")
	}
	resumed, cost, err := readPage(seq, c, 1, true)
	if err != nil || resumed.Document == nil || !resumed.Complete || resumed.Next != "" || cost != readSize(seq.document) {
		t.Fatal("first document page made no progress")
	}
}
