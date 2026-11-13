//go:build integration

package fixture

import (
	"context"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/internal/embedding"
	"danny.vn/trachuan/internal/retrieval"
	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5/pgxpool"
)

type siblingEmbedder struct{}

func (siblingEmbedder) Recipe(context.Context) (embedding.Recipe, error) {
	return embedding.TestRecipe(), nil
}
func (siblingEmbedder) Embed(_ context.Context, text string) (embedding.Vector, error) {
	return embedding.Vector{Values: embedding.FakeVector(text), TokenCount: 1, RecipeHash: embedding.TestRecipe().Hash()}, nil
}

func TestCreateRecipe3_MergedSearchAndArticleRead(t *testing.T) {
	ctx := context.Background()
	name, key, err := CreateRecipe3(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if cleanupErr := Drop(ctx, name, key); cleanupErr != nil {
			t.Error(cleanupErr)
		}
	})
	admin, base, err := testpg.OpenAdministrator(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close(ctx) })
	base.Path = "/" + name
	db, err := pgxpool.New(ctx, base.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	engine, err := retrieval.Open(ctx, db, siblingEmbedder{}, retrieval.Options{Now: func() time.Time { return time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC) }, Profile: "evaluation"})
	if err != nil {
		t.Fatal(err)
	}
	if engine.Release().RetrievalRecipe.Version != 3 {
		t.Fatal("fixture recipe")
	}
	response, err := engine.Search(ctx, retrieval.Request{Query: contextPrefix + "\n" + siblingTexts()[1], Limit: 20, Mode: "exact"})
	if err != nil {
		t.Fatal(err)
	}
	var merged *retrieval.Hit
	for i := range response.Hits {
		if response.Hits[i].Identity.DocumentKey == "vn:fixture:siblings" {
			merged = &response.Hits[i]
		}
	}
	if merged == nil || merged.Identity.Anchor != "article-1" || merged.Complete || merged.BestMember.Anchor != "clause-2" || len(merged.Fragments) != 3 {
		t.Fatalf("merged hit %+v", merged)
	}
	snippet, _ := retrieval.Snippet(*merged, retrieval.SnippetCap)
	if !strings.Contains(snippet, "Khoản hai có dấu hiệu hợp nhất màu ngọc.") {
		t.Fatalf("snippet %s", snippet)
	}
	full, err := engine.Present(ctx, response, retrieval.DetailFull)
	if err != nil {
		t.Fatal(err)
	}
	var next string
	for _, h := range full.Hits {
		if h.Doc == "vn:fixture:siblings" {
			next = h.Next
		}
	}
	if next == "" {
		t.Fatal("missing article next")
	}
	read, err := engine.Read(ctx, retrieval.ReadRequest{Release: response.Release, Items: []retrieval.ReadItem{{Next: next}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(read.Items) != 1 || read.Items[0].Error != "" {
		t.Fatalf("read %+v", read)
	}
	var text string
	for _, u := range read.Items[0].Units {
		text += u.Text
	}
	if !strings.Contains(text, "Khoản ba chỉ có trong bài đọc đầy đủ.") {
		t.Fatal("next did not read article")
	}
}
