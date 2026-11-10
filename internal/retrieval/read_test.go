package retrieval

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode"
)

func TestRead_HostileNumberIsItemErrorFast(t *testing.T) {
	item := ReadItem{Number: "1/" + strings.Repeat(" ", 60000) + "2026/QH"}
	for mode, continued := range map[string]bool{"direct": false, "next": true} {
		t.Run(mode, func(t *testing.T) {
			start := time.Now()
			assertReadInputError(t, item, continued)
			if elapsed := time.Since(start); elapsed >= 100*time.Millisecond {
				t.Fatalf("hostile number took %s", elapsed)
			}
		})
	}
}

func TestRead_NulOrInvalidUTF8IsItemError(t *testing.T) {
	for name, value := range map[string]string{"nul": "\x00", "invalid_utf8": "\xff"} {
		for field, item := range readTextItems(value) {
			for mode, continued := range map[string]bool{"direct": false, "next": true} {
				t.Run(name+"/"+field+"/"+mode, func(t *testing.T) {
					assertReadInputError(t, item, continued)
				})
			}
		}
	}
}

func TestRead_InputBounds(t *testing.T) {
	for field, item := range map[string]ReadItem{
		"handle": {Handle: "vn:" + strings.Repeat("đ", 510)},
		"number": {Number: strings.Repeat("đ", 129)},
		"from":   {Handle: "vn:1", From: strings.Repeat("đ", 513), To: "p8"},
		"to":     {Handle: "vn:1", From: "p5", To: strings.Repeat("đ", 513)},
	} {
		for mode, continued := range map[string]bool{"direct": false, "next": true} {
			t.Run(field+"/"+mode, func(t *testing.T) {
				assertReadInputError(t, item, continued)
			})
		}
	}
	for name, item := range map[string]ReadItem{
		"handle":           {Handle: "vn:" + strings.Repeat("đ", 509)},
		"number":           {Number: strings.Repeat("đ", 128)},
		"from":             {Handle: "vn:1", From: strings.Repeat("đ", 512), To: "p8"},
		"to":               {Handle: "vn:1", From: "p5", To: strings.Repeat("đ", 512)},
		"replacement_rune": {Handle: "vn:\ufffd"},
	} {
		t.Run(name+"/valid", func(t *testing.T) {
			if !validReadItem(item) {
				t.Fatal("valid code-point boundary rejected")
			}
		})
	}
}

func readTextItems(value string) map[string]ReadItem {
	return map[string]ReadItem{
		"handle": {Handle: "vn:" + value},
		"number": {Number: value},
		"from":   {Handle: "vn:1", From: value, To: "p8"},
		"to":     {Handle: "vn:1", From: "p5", To: value},
	}
}

func readTestNext(item ReadItem) string {
	c := cursor{V: 1, Release: "20991110_1", Item: item}
	data, _ := json.Marshal(c)
	// Marshal replaces invalid UTF-8. Restore the hostile byte in crafted tokens.
	data = []byte(strings.NewReplacer(`\ufffd`, "\xff", "\ufffd", "\xff").Replace(string(data)))
	return base64.RawURLEncoding.EncodeToString(data)
}

func assertReadInputError(t *testing.T, item ReadItem, continued bool) {
	t.Helper()
	defer func() {
		if p := recover(); p != nil {
			t.Fatalf("invalid input reached the nil database: %v", p)
		}
	}()
	want := "invalid_item"
	if continued {
		item = ReadItem{Next: readTestNext(item)}
		want = "invalid_next"
	}
	// A nil database proves validation precedes even transaction creation.
	e := &Engine{release: Release{Rel: "20991110_1"}}
	got, err := e.Read(context.Background(), ReadRequest{Release: e.release.Rel, Items: []ReadItem{item}})
	if err != nil || len(got.Items) != 1 || got.Items[0].Error != want {
		t.Fatalf("Read() = %+v, %v; want %s", got, err, want)
	}
}

func TestReadParts_LosslessAndPositive(t *testing.T) {
	for _, text := range []string{"", "a", strings.Repeat("điều kiện \n", 7000), strings.Repeat("á", 40000)} {
		parts, ok := unitParts(Unit{Kind: "provision", Text: text})
		if !ok || len(parts) == 0 {
			t.Fatal("valid unit rejected")
		}
		var joined strings.Builder
		for _, p := range parts {
			if readSize(p) > PartSize || (text != "" && p.Text == "") {
				t.Fatal("empty or oversized part")
			}
			joined.WriteString(p.Text)
		}
		if joined.String() != text {
			t.Fatal("parts lost source text")
		}
	}
}

func TestReadParts_NoWhitespaceKeepsCombiningMark(t *testing.T) {
	for _, tc := range []struct {
		prefix string
		width  int
	}{{"a", 1}, {"\"", 2}} {
		u := Unit{Kind: "provision", Part: 1, Parts: 2}
		capacity := PartSize - readSize(u) - len(`,"text":""`)
		prefix := strings.Repeat(tc.prefix, (capacity-1)/tc.width) + strings.Repeat("a", (capacity-1)%tc.width)
		u.Part, u.Parts = 0, 0
		u.Text = prefix + "e\u0301" + strings.Repeat("b", 20)
		parts, ok := unitParts(u)
		if !ok || len(parts) != 2 || parts[0].Text != prefix || !strings.HasPrefix(parts[1].Text, "e\u0301") || readSize(parts[0]) > PartSize {
			t.Fatal("split separated a combining mark at the encoded boundary")
		}
	}
}

func TestReadParts_FixedBoundaries(t *testing.T) {
	u := Unit{Kind: "provision", Text: strings.Repeat("văn bản pháp luật\n", 3000)}
	a, aOK := unitParts(u)
	b, bOK := unitParts(u)
	if !aOK || !bOK || len(a) < 2 || !reflect.DeepEqual(a, b) {
		t.Fatal("boundaries changed")
	}
	for i, p := range a[:len(a)-1] {
		r := []rune(p.Text)
		next := []rune(a[i+1].Text)
		if len(r) == 0 || len(next) == 0 || (!unicode.IsSpace(r[len(r)-1]) && !unicode.IsSpace(next[0])) || readSize(p) > PartSize {
			t.Fatal("did not cut at whitespace")
		}
	}
}

func TestReadParts_SerializedBudgetAndLossless(t *testing.T) {
	for _, u := range []Unit{
		{Kind: "provision", Anchor: "p1", Text: strings.Repeat("\"\\\n<&e\u0301 ", 6000)},
		{Kind: "outline", Anchor: "p1", Heading: strings.Repeat("đ", 40000), Label: "Điều 1"},
		{Kind: "relation", Relation: &ListedRelation{RawLabel: "raw label", Evidence: strings.Repeat("evidence ", 10000), EvidenceComplete: true}},
	} {
		parts, ok := unitParts(u)
		if !ok {
			t.Fatal("valid unit rejected")
		}
		var text, heading, evidence strings.Builder
		for i, p := range parts {
			if readSize(p) > PartSize {
				t.Fatalf("serialized part %d exceeds budget: %d", i, readSize(p))
			}
			if p.Part != i+1 || p.Parts != len(parts) {
				t.Fatal("part numbering")
			}
			text.WriteString(p.Text)
			heading.WriteString(p.Heading)
			if p.Relation != nil {
				evidence.WriteString(p.Relation.Evidence)
				if p.Relation.RawLabel != u.Relation.RawLabel {
					t.Fatal("raw label changed")
				}
			}
		}
		if text.String() != u.Text {
			t.Fatal("lost text")
		}
		if u.Kind == "outline" && heading.String() != u.Heading {
			t.Fatal("lost heading")
		}
		if u.Relation != nil && evidence.String() != u.Relation.Evidence {
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

func TestReadParts_OnlySplitFieldIsCut(t *testing.T) {
	for _, kind := range []string{"lead", "provision", "outline", "relation"} {
		t.Run(kind, func(t *testing.T) {
			original := Unit{Kind: kind, Anchor: "p1", Citation: "Điều 1", Label: "label", Heading: "short heading", Text: "duty"}
			source := strings.Repeat("source \"\\\ne\u0301 ", 5000)
			switch kind {
			case "lead", "provision":
				original.Text = source
			case "outline":
				original.Heading = source
			case "relation":
				original.Relation = &ListedRelation{Pointer: Pointer{Number: "number", InRelease: true}, RawLabel: "raw label", Evidence: source, EvidenceComplete: true}
			}
			seq := readSequence{units: []Unit{original}, complete: true}
			c := cursor{V: 1, Release: "release", Item: ReadItem{Handle: "vn:1#p1"}}
			var joined strings.Builder
			count := 0
			for page := 0; page < 20; page++ {
				result, _, err := readPage(seq, c, ReadCallCap, true)
				if err != nil || result.Error != "" {
					t.Fatalf("parts failed: %s %v", result.Error, err)
				}
				for _, part := range result.Units {
					if readSize(part) > PartSize {
						t.Fatal("oversized serialized part")
					}
					count++
					if part.Part != count {
						t.Fatal("part numbering")
					}
					part.Part, part.Parts = 0, 0
					switch kind {
					case "lead", "provision":
						joined.WriteString(part.Text)
						part.Text = original.Text
					case "outline":
						joined.WriteString(part.Heading)
						part.Heading = original.Heading
					case "relation":
						joined.WriteString(part.Relation.Evidence)
						r := *part.Relation
						r.Evidence = original.Relation.Evidence
						part.Relation = &r
					}
					if !reflect.DeepEqual(part, original) {
						t.Fatal("a non-split field changed")
					}
				}
				if result.Next == "" {
					if !result.Complete || count < 2 || joined.String() != source {
						t.Fatal("split field was repeated or lost")
					}
					return
				}
				c, err = decodeNext(result.Next)
				if err != nil {
					t.Fatal(err)
				}
			}
			t.Fatal("parts did not terminate")
		})
	}
}

func TestReadParts_HugeHeadingIsUnitTooLarge(t *testing.T) {
	assertUnitTooLarge(t, Unit{Kind: "provision", Heading: strings.Repeat("h", 20000), Text: "duty"})
}

func TestReadParts_OversizedPointerNumberIsUnitTooLarge(t *testing.T) {
	assertUnitTooLarge(t, Unit{Kind: "relation", Relation: &ListedRelation{Pointer: Pointer{Number: strings.Repeat("n", PartSize+1)}, Evidence: "evidence"}})
}

func TestReadParts_NoEncodedRuneFitsIsUnitTooLarge(t *testing.T) {
	base := Unit{Kind: "provision", Heading: "h", Part: 1, Parts: 1}
	base.Heading = strings.Repeat("h", 1+PartSize-readSize(base))
	if readSize(base) != PartSize {
		t.Fatal("bad boundary fixture")
	}
	base.Part, base.Parts = 0, 0
	base.Text = strings.Repeat("\"", 20)
	assertUnitTooLarge(t, base)
}

func assertUnitTooLarge(t *testing.T, u Unit) {
	t.Helper()
	seq := readSequence{units: []Unit{u}, complete: true}
	result, cost, err := readPage(seq, cursor{V: 1, Release: "release", Item: ReadItem{Handle: "vn:1"}}, ReadCallCap, true)
	if err != nil || result.Error != "unit_too_large" || result.Complete || result.Next != "" || result.NotRead != "" || len(result.Units) != 0 || cost != 0 {
		t.Fatalf("expected terminal unit error: error=%q, units=%d, complete=%t, next=%t, not_read=%q, cost=%d, err=%v", result.Error, len(result.Units), result.Complete, result.Next != "", result.NotRead, cost, err)
	}
}

func TestReadPage_ContinuationSequenceSameAcrossBudgets(t *testing.T) {
	seq := readSequence{units: []Unit{{Kind: "provision", Text: strings.Repeat("a", 100000)}}, complete: true}
	start := cursor{V: 1, Release: "release", Item: ReadItem{Handle: "vn:1#p1"}, Part: 1}
	collect := func(budget int) ([]Unit, int) {
		t.Helper()
		c := start
		var units []Unit
		firstCount := 0
		for page := 0; page < 20; page++ {
			result, cost, err := readPage(seq, c, budget, false)
			if err != nil || result.Error != "" || cost > budget {
				t.Fatal("bad page")
			}
			if page == 0 {
				firstCount = len(result.Units)
			}
			units = append(units, result.Units...)
			if result.Next == "" {
				if !result.Complete {
					t.Fatal("incomplete chain")
				}
				return units, firstCount
			}
			c, err = decodeNext(result.Next)
			if err != nil {
				t.Fatal(err)
			}
		}
		t.Fatal("chain did not terminate")
		return nil, 0
	}
	alone, aloneCount := collect(ReadCallCap)
	reduced, reducedCount := collect(20000)
	if aloneCount <= reducedCount || !reflect.DeepEqual(alone, reduced) {
		t.Fatal("continuation sequence changed with the budget")
	}
}

func TestRead_ItemDocAndAnchors(t *testing.T) {
	p := Provision{Identity: Identity{DocumentKey: "vn:1", Anchor: "article"}, Kind: "dieu", Text: "text"}
	u := readTextUnit("provision", p, []Provision{p}, "text")
	b, err := json.Marshal(u)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"anchor":"article"`) || strings.Contains(string(b), `"handle"`) {
		t.Fatalf("unit JSON: %s", b)
	}
	seq := readSequence{doc: "vn:1", units: []Unit{u}, complete: true}
	result, _, err := readPage(seq, cursor{}, ReadCallCap, true)
	if err != nil || result.Doc != "vn:1" || !result.Complete {
		t.Fatalf("item: %+v, %v", result, err)
	}
}
