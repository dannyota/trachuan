package webapi

import (
	"context"
	"strings"
	"testing"

	"danny.vn/trachuan/internal/retrieval"
)

type populatedEngine struct{ *testEngine }

func (e *populatedEngine) Present(_ context.Context, _ retrieval.Response, detail retrieval.Detail) (retrieval.SearchResult, error) {
	score := 0.7
	locator := "block:2-4"
	doc := retrieval.DocumentEntry{Title: "Example", TextSource: "html_transcription", TextComplete: false, Status: retrieval.Status{Current: true}, Related: []retrieval.Pointer{{Dir: "in", Kind: "amends_supplements", Key: "vn:2", InRelease: true}}, RelatedCounts: map[string]int{"in:amends_supplements": 1}}
	hit := retrieval.PresentedHit{Locator: &locator, Handle: "vn:1#a", Doc: "vn:1", Citation: "Article 1", Path: []retrieval.PresentedPathNode{{Anchor: "a", Label: "Article 1"}}, Text: "source", Complete: true, Score: &score,
		Fragments: []retrieval.Fragment{{Identity: retrieval.Identity{Country: "vn", DocumentKey: "vn:1", Anchor: "a"}, Start: 1, End: 2, TextStart: 1, TextEnd: 2, Role: "body", Complete: true}},
		Relations: []retrieval.Relation{{SourceKey: "vn:1", Kind: "cites", Evidence: "evidence", Complete: true, SourceResolved: true}}, Warnings: []retrieval.Warning{{Pointer: doc.Related[0], Basis: "text", Target: "vn:1#a", Citation: "Article 1", Exact: true}}, WarningsOmitted: 1,
	}
	if detail == retrieval.DetailCompact {
		hit.Snippet = "source"
		hit.Text = ""
		hit.Fragments = nil
		hit.Relations = nil
		hit.Score = nil
	}
	return retrieval.SearchResult{Release: "20991110_1", Documents: map[string]retrieval.DocumentEntry{"vn:1": doc}, Hits: []retrieval.PresentedHit{hit}}, nil
}

func (e *populatedEngine) Read(_ context.Context, req retrieval.ReadRequest) (retrieval.ReadResult, error) {
	doc := retrieval.DocumentEntry{Title: "Example", TextSource: "html_transcription", TextComplete: false, Status: retrieval.Status{Current: true}}
	locator := "block:2-4"
	item := retrieval.ItemResult{FetchSHA256: strings.Repeat("a", 64), Doc: "vn:1", Document: &doc, Units: []retrieval.Unit{{Kind: "provision", Locator: &locator, Anchor: "a", Text: "source", Warnings: []retrieval.Warning{{Pointer: retrieval.Pointer{InRelease: true}, Basis: "text", Target: "vn:1#a", Citation: "Article 1", Exact: true}}, WarningsOmitted: 1, Part: 1, Parts: 2, Relation: &retrieval.ListedRelation{Pointer: retrieval.Pointer{Dir: "in", Kind: "cites", InRelease: true}, Evidence: "evidence", EvidenceComplete: true}}}, Next: "continuation"}
	return retrieval.ReadResult{Release: req.Release, Items: []retrieval.ItemResult{item}}, nil
}

func TestPopulatedContract_TextMetadataAndLocator(t *testing.T) {
	for _, tc := range []struct{ method, path, body string }{
		{"POST", "/api/v1/search", `{"country":"vn","query":"ngân hàng","detail":"full"}`},
		{"POST", "/api/v1/read", `{"country":"vn","items":[{"handle":"vn:1"}]}`},
		{"GET", "/api/v1/documents/vn:1", ""},
	} {
		t.Run(tc.path, func(t *testing.T) {
			h := newHarness(t)
			h.resolver.engine = &populatedEngine{testEngine: h.engine}
			w := webCall(h, tc.method, tc.path, tc.body)
			if w.Code != 200 {
				t.Fatalf("status %d: %s", w.Code, w.Body.String())
			}
			for _, want := range []string{`"text_source":"html_transcription"`, `"text_complete":false`, `"locator":"block:2-4"`} {
				if !strings.Contains(w.Body.String(), want) {
					t.Errorf("missing %s in %s", want, w.Body.String())
				}
			}
		})
	}
}

func TestDocumentPage_FetchHashAndMCPIsolation(t *testing.T) {
	h := newHarness(t)
	h.resolver.engine = &populatedEngine{testEngine: h.engine}
	w := webCall(h, "GET", "/api/v1/documents/vn:1?release=20991110_1", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"fetch_sha256":"`+strings.Repeat("a", 64)+`"`) {
		t.Fatal(w.Code, w.Body.String())
	}
	read := mcpCall(t, h, "read", `{"country":"vn","release":"20991110_1","items":[{"handle":"vn:1"}]}`)
	if strings.Contains(string(read), `"fetch_sha256"`) {
		t.Fatal("fetch hash leaked to MCP")
	}
}
