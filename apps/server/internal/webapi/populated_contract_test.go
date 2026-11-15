package webapi

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/contracttest"
	"danny.vn/trachuan/internal/retrieval"
)

type populatedEngine struct {
	*testEngine
	files     []retrieval.SourceFile
	itemError string
}

func (e *populatedEngine) Present(_ context.Context, _ retrieval.Response, detail retrieval.Detail) (retrieval.SearchResult, error) {
	score := 0.7
	locator := "block:2-4"
	doc := retrieval.DocumentEntry{Title: "Example", TextSource: "pdf_text", TextComplete: false, Status: retrieval.Status{Current: true}, Related: []retrieval.Pointer{{Dir: "in", Kind: "amends_supplements", Key: "vn:2", InRelease: true}}, RelatedCounts: map[string]int{"in:amends_supplements": 1}}
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
	doc := retrieval.DocumentEntry{Title: "Example", TextSource: "pdf_text", TextComplete: false, Status: retrieval.Status{Current: true}}
	locator := "block:2-4"
	item := retrieval.ItemResult{Files: e.files, Error: e.itemError, FetchSHA256: strings.Repeat("a", 64), Doc: "vn:1", Document: &doc, Units: []retrieval.Unit{{Kind: "provision", Locator: &locator, Anchor: "a", Text: "source", Warnings: []retrieval.Warning{{Pointer: retrieval.Pointer{InRelease: true}, Basis: "text", Target: "vn:1#a", Citation: "Article 1", Exact: true}}, WarningsOmitted: 1, Part: 1, Parts: 2, Relation: &retrieval.ListedRelation{Pointer: retrieval.Pointer{Dir: "in", Kind: "cites", InRelease: true}, Evidence: "evidence", EvidenceComplete: true}}}, Next: "continuation"}
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
			for _, want := range []string{`"text_source":"pdf_text"`, `"text_complete":false`, `"locator":"block:2-4"`} {
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
	if !strings.Contains(w.Body.String(), `"files":[]`) {
		t.Fatal("missing empty files array", w.Body.String())
	}
	read := mcpCall(t, h, "read", `{"country":"vn","release":"20991110_1","items":[{"handle":"vn:1"}]}`)
	if strings.Contains(string(read), `"fetch_sha256"`) || strings.Contains(string(read), `"files"`) {
		t.Fatal("fetch hash leaked to MCP")
	}
}

func TestDocumentPage_SourceFilesAndErrors(t *testing.T) {
	contract, err := contracttest.Load()
	if err != nil {
		t.Fatal(err)
	}
	coverage := contracttest.NewCoverageForTag(contract, "web")
	url := "https://example.invalid/law.pdf"
	fetched := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	files := []retrieval.SourceFile{
		{Name: "law.pdf", Hash: strings.Repeat("b", 64), Size: 9007199254740993, Role: "unknown"},
		{Name: "scan.pdf", URL: &url, Hash: strings.Repeat("c", 64), Size: 42, Role: "scan", FetchedAt: &fetched},
	}
	for _, code := range []string{"", "not_found", "invalid_next", "unit_too_large"} {
		t.Run(code, func(t *testing.T) {
			h := newHarness(t)
			h.resolver.engine = &populatedEngine{testEngine: h.engine, files: files, itemError: code}
			for _, suffix := range []string{"", "&next=continuation"} {
				w := webCall(h, "GET", "/api/v1/documents/vn:1?release=20991110_1"+suffix, "")
				if w.Code != 200 {
					t.Fatal(w.Code, w.Body.String())
				}
				coverage.Check(t, "GET", "/api/v1/documents/{key}", w)
				var page map[string]json.RawMessage
				if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
					t.Fatal(err)
				}
				if code != "" {
					if page["files"] != nil || page["fetch_sha256"] != nil {
						t.Fatal("error item carries source metadata", w.Body.String())
					}
					continue
				}
				var got []retrieval.SourceFile
				if err := json.Unmarshal(page["files"], &got); err != nil || !reflect.DeepEqual(got, files) {
					t.Fatalf("files = %+v, error = %v", got, err)
				}
				if !strings.Contains(string(page["files"]), `"url":null`) || !strings.Contains(string(page["files"]), `"fetched_at":null`) {
					t.Fatal("unknown provenance lost its nulls", string(page["files"]))
				}
			}
			read := mcpCall(t, h, "read", `{"country":"vn","release":"20991110_1","items":[{"handle":"vn:1"}]}`)
			if strings.Contains(string(read), `"files"`) || strings.Contains(string(read), "fetched_at") {
				t.Fatal("source files leaked to MCP")
			}
		})
	}
}
