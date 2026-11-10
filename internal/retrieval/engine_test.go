package retrieval

import (
	"context"
	"testing"

	"danny.vn/trachuan/internal/embedding"
)

type fakeEmbedder struct {
	calls int
	text  string
}

func (f *fakeEmbedder) Recipe(context.Context) (embedding.Recipe, error) {
	return embedding.QueryRecipe1(), nil
}
func (f *fakeEmbedder) Embed(_ context.Context, text string) (embedding.Vector, error) {
	f.calls++
	f.text = text
	values := make([]float32, 1024)
	values[0] = 1
	return embedding.Vector{Values: values, TokenCount: 1, RecipeHash: embedding.QueryRecipe1().Hash()}, nil
}
func TestPrepare_EmbedsOnceAndRejectsBeforeEmbeddingUnit(t *testing.T) {
	f := &fakeEmbedder{}
	e := &Engine{embedder: f, queryRecipe: embedding.QueryRecipe1()}
	p, err := e.Prepare(context.Background(), Request{Query: "ngan hang"})
	if err != nil || p.Validation.Accepted || f.calls != 0 {
		t.Fatalf("%+v %v %d", p, err, f.calls)
	}
	q := "  ngân hàng  "
	p, err = e.Prepare(context.Background(), Request{Query: q})
	if err != nil || p.Vector == nil || f.calls != 1 || f.text != q {
		t.Fatalf("%+v %v %d", p, err, f.calls)
	}
}

func TestOpenDraft_RejectsMissingDependencies(t *testing.T) {
	_, err := OpenDraft(context.Background(), nil, nil, Release{}, Options{})
	if err == nil {
		t.Fatal("accepted missing dependencies")
	}
}

func TestCollect_RejectsMissingVector(t *testing.T) {
	e := &Engine{}
	_, err := e.Collect(context.Background(), Prepared{Validation: Validation{Accepted: true}}, Depths{Dense: 1, Lexical: 1})
	if err == nil {
		t.Fatal("accepted missing vector")
	}
}

func TestLoadDecisions_EmptyHits(t *testing.T) {
	var engine Engine
	if err := engine.LoadDecisions(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
}
