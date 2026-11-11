package webapi

import (
	"context"

	"danny.vn/trachuan/internal/retrieval"
)

type populatedEngine struct{ *testEngine }

func (e *populatedEngine) Present(context.Context, retrieval.Response, retrieval.Detail) (retrieval.SearchResult, error) {
	score := 0.7
	doc := retrieval.DocumentEntry{Title: "Example", Status: retrieval.Status{Current: true}, Related: []retrieval.Pointer{{Dir: "in", Kind: "amends_supplements", Key: "vn:2", InRelease: true}}, RelatedCounts: map[string]int{"in:amends_supplements": 1}}
	hit := retrieval.PresentedHit{Handle: "vn:1#a", Doc: "vn:1", Citation: "Article 1", Path: []retrieval.PresentedPathNode{{Anchor: "a", Label: "Article 1"}}, Text: "source", Complete: true, Score: &score,
		Fragments: []retrieval.Fragment{{Identity: retrieval.Identity{Country: "vn", DocumentKey: "vn:1", Anchor: "a"}, Start: 1, End: 2, TextStart: 1, TextEnd: 2, Role: "body", Complete: true}},
		Relations: []retrieval.Relation{{SourceKey: "vn:1", Kind: "cites", Evidence: "evidence", Complete: true, SourceResolved: true}}, Warnings: doc.Related,
	}
	return retrieval.SearchResult{Release: "20991110_1", Documents: map[string]retrieval.DocumentEntry{"vn:1": doc}, Hits: []retrieval.PresentedHit{hit}}, nil
}

func (e *populatedEngine) Read(_ context.Context, req retrieval.ReadRequest) (retrieval.ReadResult, error) {
	doc := retrieval.DocumentEntry{Title: "Example", Status: retrieval.Status{Current: true}}
	item := retrieval.ItemResult{Doc: "vn:1", Document: &doc, Units: []retrieval.Unit{{Kind: "provision", Anchor: "a", Text: "source", Part: 1, Parts: 2, Relation: &retrieval.ListedRelation{Pointer: retrieval.Pointer{Dir: "in", Kind: "cites", InRelease: true}, Evidence: "evidence", EvidenceComplete: true}}}, Next: "continuation"}
	return retrieval.ReadResult{Release: req.Release, Items: []retrieval.ItemResult{item}}, nil
}
