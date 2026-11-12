//go:build integration

package retrieval

import (
	"context"
	"reflect"
	"testing"
)

func TestPagedHits_EqualsSearchPages(t *testing.T) {
	e := taxonomyFixture(t)
	ctx := context.Background()
	req := Request{Query: "ngân hàng", Limit: 2, Mode: "exact", Arms: "hybrid", Nodes: []string{"1.1"}}
	prepared, err := e.Prepare(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	recipe := e.Release().RetrievalRecipe
	arms, err := e.Collect(ctx, prepared, Depths{Dense: recipe.DenseDepth, Lexical: recipe.LexicalDepth})
	if err != nil {
		t.Fatal(err)
	}
	pages, err := PagedHits(arms, recipe, req, 2, 100)
	if err != nil {
		t.Fatal(err)
	}
	for i, page := range pages {
		if err = e.LoadDecisions(ctx, page.Hits, page.NonCurrent); err != nil {
			t.Fatal(err)
		}
		want, searchErr := e.Search(ctx, req)
		if searchErr != nil {
			t.Fatal(searchErr)
		}
		if !reflect.DeepEqual(page, want) {
			t.Fatalf("page %d differs from Search", i)
		}
		req.Cursor = want.NextCursor
		if req.Cursor == "" && i != len(pages)-1 {
			t.Fatal("pages after exhaustion")
		}
	}
	if req.Cursor != "" {
		t.Fatal("search pages omitted")
	}
}
