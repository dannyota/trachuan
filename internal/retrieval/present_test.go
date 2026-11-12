package retrieval

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func presentationHit(key, anchor string) Hit {
	return Hit{Identity: Identity{Country: "vn", DocumentKey: key, Anchor: anchor}, Citation: "Điều 1", Text: "source text", Complete: true}
}

func TestPresent_DocumentsOnce(t *testing.T) {
	a := presentationHit("vn:1", "a")
	a.Stamp = Validity{StatusClass: "in_force", Basis: "document_dates", StatusBasis: "source_status", Current: true}
	b := a
	b.Stamp = Validity{StatusClass: "expired"}
	docs := map[string]DocumentEntry{"vn:1": {Title: "Title"}, "vn:unused": {Title: "Unused"}}
	got := present(Response{Hits: []Hit{a, b, b}}, docs, DetailCompact)
	if len(got.Documents) != 1 || got.Documents["vn:1"].Status != (Status{Class: "in_force", Basis: "document_dates", StatusBasis: "source_status", Current: true}) {
		t.Fatalf("documents: %+v", got.Documents)
	}
	if docs["vn:1"].Status.Current {
		t.Fatal("input map was modified")
	}
}

func TestPresent_CompactHitFields(t *testing.T) {
	h := presentationHit("vn:1", "article/clause")
	h.Path = []PathNode{{Identity: Identity{DocumentKey: "vn:1", Anchor: "article"}, Label: "Article", Heading: "Heading"}, {Identity: h.Identity, Label: "Clause"}}
	h.Text = strings.Repeat("word ", 150)
	h.Score = 0.5
	h.Relations = []Relation{{Kind: "cites"}}
	got := present(Response{Hits: []Hit{h}}, map[string]DocumentEntry{"vn:1": {}}, DetailCompact).Hits[0]
	snippet, complete := Snippet(h, SnippetCap)
	if got.Handle != "vn:1#article/clause" || got.Doc != "vn:1" || got.Citation != "Điều 1" || got.Snippet != snippet || got.Complete != complete || len(got.Path) != 2 || got.Path[0].Anchor != "article" || got.Path[0].Heading != "Heading" || got.Path[1].Anchor != "article/clause" {
		t.Fatalf("hit: %+v", got)
	}
	b, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"text"`, `"score"`, `"relations"`, `"fragments"`, `"next"`} {
		if strings.Contains(string(b), field) {
			t.Fatalf("compact JSON: %s", b)
		}
	}
}

func TestPresent_FullHitFields(t *testing.T) {
	h := presentationHit("vn:1", "a")
	h.Complete = false
	h.Score = 0
	h.Fragments = []Fragment{{Identity: h.Identity, End: 4, TextEnd: 4, Role: "body", Complete: false}}
	h.Relations = []Relation{{Kind: "cites", Evidence: "evidence"}}
	got := present(Response{Release: "release", Hits: []Hit{h}}, map[string]DocumentEntry{"vn:1": {}}, DetailFull).Hits[0]
	if got.Text != h.Text || !reflect.DeepEqual(got.Fragments, h.Fragments) || got.Score == nil || *got.Score != 0 || !reflect.DeepEqual(got.Relations, h.Relations) || got.Complete || got.Snippet != "" {
		t.Fatalf("hit: %+v", got)
	}
	c, err := decodeNext(got.Next)
	if err != nil || c.V != 1 || c.Release != "release" || c.Item != (ReadItem{Handle: "vn:1#a"}) || c.Unit != 0 || c.Part != 0 {
		t.Fatalf("cursor: %+v, %v", c, err)
	}
	b, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), `"snippet"`) || !strings.Contains(string(b), `"score":0`) {
		t.Fatalf("full JSON: %s", b)
	}
	h.Complete = true
	if present(Response{Hits: []Hit{h}}, nil, DetailFull).Hits[0].Next != "" {
		t.Fatal("complete hit has continuation")
	}
}

func TestPresent_WarningsOnlyProvisionLevel(t *testing.T) {
	h := presentationHit("vn:1", "article")
	target, anchor := "vn:1", "article/clause"
	other, sourceAnchor := "vn:2", "source"
	h.Relations = []Relation{
		{SourceKey: "vn:2", TargetKey: &target, Kind: "repeals"},
		{SourceKey: "vn:2", TargetKey: &target, TargetAnchor: &anchor, TargetInHit: true, Kind: "repeals", SourceNumber: "two", SourceResolved: true, SourceDocKey: "vn:2"},
		{SourceKey: "vn:1", SourceAnchor: &sourceAnchor, SourceInHit: true, TargetKey: &other, TargetNumber: "two", Kind: "amends_supplements", TargetResolved: false},
		{SourceKey: "vn:2", TargetKey: &target, TargetAnchor: &anchor, TargetInHit: true, Kind: "cites"},
	}
	got := present(Response{Hits: []Hit{h}}, nil, DetailCompact).Hits[0].Warnings
	want := []Pointer{{Dir: "in", Kind: "repeals", Key: "vn:2", Number: "two", InRelease: true}, {Dir: "out", Kind: "amends_supplements", Key: "vn:2", Number: "two"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("warnings: %+v", got)
	}
}

func TestPresent_IntraDocumentWarnings(t *testing.T) {
	key, source, target := "vn:1", "article-a", "article-b"
	relation := Relation{SourceKey: key, TargetKey: &key, SourceAnchor: &source, TargetAnchor: &target, SourceNumber: "one", TargetNumber: "one", Kind: "repeals", SourceResolved: true, TargetResolved: true, SourceDocKey: key, TargetDocKey: key}
	for _, tc := range []struct{ anchor, direction string }{{source, "out"}, {target, "in"}} {
		t.Run(tc.direction, func(t *testing.T) {
			h := presentationHit(key, tc.anchor)
			r := relation
			r.SourceInHit = tc.direction == "out"
			r.TargetInHit = tc.direction == "in"
			h.Relations = []Relation{r}
			warnings := present(Response{Hits: []Hit{h}}, nil, DetailCompact).Hits[0].Warnings
			if len(warnings) != 1 || warnings[0].Dir != tc.direction {
				t.Fatalf("warnings: %+v, want only %s", warnings, tc.direction)
			}
		})
	}
}

func TestPresent_RelationFlagsDoNotChangeJSON(t *testing.T) {
	r := Relation{SourceKey: "vn:1", Kind: "repeals"}
	without, _ := shapeRelations([]SourceRelation{{Value: r}}, Recipe1(), Identity{})
	r.SourceDocKey, r.TargetDocKey = "vn:1", "vn:2"
	with, _ := shapeRelations([]SourceRelation{{Value: r, SourceInHit: true, TargetInHit: true}}, Recipe1(), Identity{})
	if len(with) != 1 || !with[0].SourceInHit || !with[0].TargetInHit {
		t.Fatalf("shaped flags: %+v", with)
	}
	before, err := json.Marshal(without[0])
	if err != nil {
		t.Fatal(err)
	}
	after, err := json.Marshal(with[0])
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatalf("relation JSON changed: %s, %s", before, after)
	}
}

func TestPresent_JSONOmitsEmptyKeepsBooleans(t *testing.T) {
	h := presentationHit("vn:1", "a")
	h.Complete = false
	h.Text = ""
	got := present(Response{Release: "r", Hits: []Hit{h}}, map[string]DocumentEntry{"vn:1": {}}, DetailCompact)
	b, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"release":"r","abstain":false,"documents":{"vn:1":{"status":{"current":false},"text_source":"","text_complete":false}},"hits":[{"handle":"vn:1#a","doc":"vn:1","citation":"Điều 1","complete":false,"nodes":[]}],"nodes":{}}`
	if string(b) != want {
		t.Fatalf("JSON: %s\nwant: %s", b, want)
	}
}

func TestPresent_NonCurrentDocumentsIncluded(t *testing.T) {
	a, b := presentationHit("vn:1", "a"), presentationHit("vn:2", "b")
	b.Stamp = Validity{StatusClass: "expired"}
	got := present(Response{Hits: []Hit{a}, NonCurrent: []Hit{b}}, map[string]DocumentEntry{"vn:1": {}, "vn:2": {Title: "two"}}, DetailCompact)
	if len(got.Documents) != 2 || got.Documents["vn:2"].Title != "two" || got.Documents["vn:2"].Status.Class != "expired" || len(got.NonCurrent) != 1 || got.NonCurrent[0].Doc != "vn:2" {
		t.Fatalf("result: %+v", got)
	}
}

func TestDefaultLimit(t *testing.T) {
	for _, tc := range []struct {
		detail Detail
		want   int
	}{{DetailCompact, 20}, {DetailFull, 8}, {"", 20}} {
		if got := DefaultLimit(tc.detail); got != tc.want {
			t.Errorf("DefaultLimit(%q) = %d, want %d", tc.detail, got, tc.want)
		}
	}
}

func TestPresent_PathNodesUseAnchors(t *testing.T) {
	h := presentationHit("vn:1", "article/clause")
	h.Path = []PathNode{{Identity: Identity{DocumentKey: "vn:1", Anchor: "article"}, Label: "Article"}}
	b, err := json.Marshal(present(Response{Hits: []Hit{h}}, nil, DetailCompact).Hits[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"path":[{"label":"Article","anchor":"article"}]`) {
		t.Fatalf("path JSON: %s", b)
	}
}

func TestPresent_WarningUsesResolvedDocumentKey(t *testing.T) {
	h := presentationHit("vn:1", "a")
	stored := "vn:stale"
	h.Relations = []Relation{
		{SourceKey: "vn:stale", SourceDocKey: "vn:2", SourceResolved: true, TargetInHit: true, Kind: "repeals"},
		{TargetKey: &stored, TargetDocKey: "vn:3", TargetResolved: true, SourceInHit: true, Kind: "repeals"},
		{TargetDocKey: "vn:2", TargetResolved: true, SourceInHit: true, Kind: "repeals"},
	}
	got := warningPointers(h)
	if len(got) != 3 || got[0].Key != "vn:2" || got[1].Key != "vn:3" || got[2].Key != "vn:2" {
		t.Fatalf("warnings: %+v", got)
	}
}

func TestHit_DocumentEntryCarriesTextSource(t *testing.T) {
	doc := DocumentEntry{TextSource: "html_transcription", TextComplete: false}
	b, err := json.Marshal(doc)
	if err != nil || !strings.Contains(string(b), `"text_source":"html_transcription"`) || !strings.Contains(string(b), `"text_complete":false`) {
		t.Fatalf("document %s: %v", b, err)
	}
}
func TestHit_LocatorOnlyForGeneratedText(t *testing.T) {
	locator := "block:1-3"
	h := presentationHit("vn:1", "a")
	h.Locator = &locator
	result := present(Response{Hits: []Hit{h}}, nil, DetailCompact)
	b, err := json.Marshal(result.Hits[0])
	if err != nil || !strings.Contains(string(b), `"locator":"block:1-3"`) {
		t.Fatalf("hit %s: %v", b, err)
	}
	h.Locator = nil
	b, err = json.Marshal(present(Response{Hits: []Hit{h}}, nil, DetailCompact).Hits[0])
	if err != nil || strings.Contains(string(b), `"locator"`) {
		t.Fatalf("hit %s: %v", b, err)
	}
}
