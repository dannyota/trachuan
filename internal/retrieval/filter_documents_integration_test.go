//go:build integration

package retrieval

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type filterDocumentTrace struct {
	filters                    int
	filterKeys                 []string
	metadata, relations, nodes [][]string
}

func (trace *filterDocumentTrace) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if len(data.Args) == 0 {
		return ctx
	}
	keys, ok := data.Args[0].([]string)
	if !ok {
		return ctx
	}
	keys = slices.Clone(keys)
	slices.Sort(keys)
	keys = slices.Compact(keys)
	switch {
	case data.SQL == `SELECT key,doc_type,issuer FROM document WHERE key=ANY($1::text[])`:
		trace.filters++
		trace.filterKeys = keys
	case strings.HasPrefix(data.SQL, `SELECT key, number, title, doc_type, issuer,`):
		trace.metadata = append(trace.metadata, keys)
	case data.SQL == documentCountsSQL || data.SQL == documentPointersSQL:
		trace.relations = append(trace.relations, keys)
	case strings.HasPrefix(data.SQL, "WITH RECURSIVE") && strings.Contains(data.SQL, "count(DISTINCT e.origin)"):
		trace.nodes = append(trace.nodes, keys)
	}
	return ctx
}

func (*filterDocumentTrace) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func TestSearch_FilterLoadsOnlyCandidateFacets(t *testing.T) {
	for _, tc := range []struct {
		name           string
		types, issuers []string
		outside        int
	}{
		{name: "type", types: []string{" Café "}, outside: 2},
		{name: "issuer", issuers: []string{" Cơ quan "}, outside: 1},
		{name: "both", types: []string{" Café "}, issuers: []string{" Cơ quan "}, outside: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := taxonomyFixture(t)
			ctx := t.Context()
			_, err := e.db.Exec(ctx, `UPDATE document SET doc_type=CASE id WHEN 1 THEN $1 ELSE 'law' END,issuer=CASE id WHEN 2 THEN 'other' ELSE $2 END`, " Cafe\u0301 ", " Co\u031b quan ")
			if err != nil {
				t.Fatal(err)
			}
			insertDocumentRelation(t, e.db, 1, 1, 2, "vn:1", "vn:2", "one", "two", "cites", strings.Repeat("a", 64), 0)
			req := Request{Query: "ngân hàng", Limit: 1, Mode: "exact", Arms: "dense"}
			baseline, err := e.Search(ctx, req)
			if err != nil {
				t.Fatal(err)
			}
			baselinePage, err := e.Present(ctx, baseline, DetailFull)
			if err != nil {
				t.Fatal(err)
			}
			if len(baseline.Hits) != 1 || baseline.Hits[0].Identity.DocumentKey != "vn:1" || len(baseline.NonCurrent) != 0 {
				t.Fatal("unexpected first-page fixture", pageIdentities(baseline))
			}
			if len(baselinePage.Documents["vn:1"].Nodes) == 0 || len(baselinePage.Documents["vn:1"].Related) == 0 {
				t.Fatal("fixture needs document nodes and relations")
			}

			trace := &filterDocumentTrace{}
			cfg := e.db.Config()
			cfg.ConnConfig.Tracer = trace
			db, err := pgxpool.NewWithConfig(ctx, cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			e.db = db
			req.DocTypes, req.Issuers = tc.types, tc.issuers
			got, err := e.Search(ctx, req)
			if err != nil {
				t.Fatal(err)
			}
			if len(trace.metadata)+len(trace.relations)+len(trace.nodes) != 0 {
				t.Fatalf("search loaded full candidate documents: metadata=%v relations=%v nodes=%v", trace.metadata, trace.relations, trace.nodes)
			}
			if trace.filters != 1 || !reflect.DeepEqual(trace.filterKeys, []string{"vn:1", "vn:2", "vn:3"}) {
				t.Fatalf("filter batches=%d keys=%v", trace.filters, trace.filterKeys)
			}
			if got.OutsideFilterCount == nil || *got.OutsideFilterCount != tc.outside || got.Hits[0].InFilter == nil || !*got.Hits[0].InFilter {
				t.Fatal("filter result changed", got)
			}
			if got.NextCursor == "" {
				t.Fatal("off-page candidates are not reachable")
			}
			want := baseline
			yes := true
			want.Hits[0].InFilter = &yes
			want.OutsideFilterCount = &tc.outside
			// The cursor digest intentionally differs when filters are present.
			want.NextCursor = got.NextCursor
			if !reflect.DeepEqual(got, want) {
				t.Fatal("filter changed the returned page")
			}

			page, err := e.Present(ctx, got, DetailFull)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(page.Documents, baselinePage.Documents) {
				t.Fatal("returned document metadata changed")
			}
			if len(trace.metadata) != 1 || len(trace.relations) != 2 || len(trace.nodes) != 1 {
				t.Fatalf("presentation did not exercise full metadata loading: metadata=%v relations=%v nodes=%v", trace.metadata, trace.relations, trace.nodes)
			}
			for _, batches := range [][][]string{trace.metadata, trace.relations, trace.nodes} {
				for _, keys := range batches {
					if !reflect.DeepEqual(keys, []string{"vn:1"}) {
						t.Fatal("presentation loaded off-page documents", keys)
					}
				}
			}
		})
	}
}
