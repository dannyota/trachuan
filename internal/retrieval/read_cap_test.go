package retrieval

import (
	"strings"
	"testing"
)

func TestReadPage_OversizedDocumentAndFirstUnitIsTerminal(t *testing.T) {
	for _, titleLength := range []int{ReadCallCap, ReadCallCap - 100} {
		for _, first := range []bool{true, false} {
			seq := readSequence{doc: "vn:1", document: &DocumentEntry{Title: strings.Repeat("x", titleLength)}, units: []Unit{{Kind: "outline", Heading: strings.Repeat("h", 200)}}, complete: true}
			c := cursor{V: 1, Release: "20990101", Item: ReadItem{Handle: "vn:1"}}
			result, cost, err := readPage(seq, c, ReadCallCap, first)
			if err != nil || result.Error != "unit_too_large" || result.Next != "" || result.Document != nil || len(result.Units) != 0 || result.Complete || cost != 0 {
				t.Fatal("oversized metadata escaped the call cap", cost, result.Error)
			}
		}
	}
}

func TestReadPage_OversizedEmptyDocumentIsTerminal(t *testing.T) {
	seq := readSequence{doc: "vn:1", document: &DocumentEntry{Title: strings.Repeat("x", ReadCallCap)}, complete: true}
	result, cost, err := readPage(seq, cursor{}, ReadCallCap, true)
	if err != nil || result.Error != "unit_too_large" || result.Next != "" || cost > ReadCallCap || result.Document != nil {
		t.Fatal("oversized empty document escaped the cap")
	}
}

func TestReadPage_DocumentAndFirstPartFitsExactly(t *testing.T) {
	unit := Unit{Kind: "outline", Heading: strings.Repeat("h", PartSize*2)}
	parts, ok := unitParts(unit)
	if !ok {
		t.Fatal("bad test unit")
	}
	doc := &DocumentEntry{Title: "x"}
	overhead := readSize(doc) - 1
	doc.Title = strings.Repeat("x", ReadCallCap-overhead-readSize(parts[0]))
	seq := readSequence{doc: "vn:1", document: doc, units: []Unit{unit}, complete: true}
	c := cursor{V: 1, Release: "20990101", Item: ReadItem{Handle: "vn:1"}}
	result, cost, err := readPage(seq, c, ReadCallCap, true)
	if err != nil || result.Error != "" || cost != ReadCallCap || len(result.Units) != 1 || result.Next == "" {
		t.Fatal("exactly fitting first part was rejected", cost, result.Error)
	}
	deferred, cost, err := readPage(seq, c, ReadCallCap-1, false)
	if err != nil || deferred.NotRead != "call_cap" || deferred.Next == "" || deferred.Document != nil || cost != 0 {
		t.Fatal("later item did not defer")
	}
}

func TestReadPage_OversizedDocumentStopsBeforeParts(t *testing.T) {
	parts := make(map[int][]Unit)
	seq := readSequence{doc: "vn:1", document: &DocumentEntry{Title: strings.Repeat("x", ReadCallCap)}, units: []Unit{{Kind: "outline", Heading: "heading"}}, parts: parts}
	result, _, err := readPage(seq, cursor{}, ReadCallCap, true)
	if err != nil || result.Error != "unit_too_large" || len(parts) != 0 {
		t.Fatal("oversized document prepared unit parts")
	}
}
