package retrieval

import (
	"context"
	"reflect"
	"testing"
)

func TestCollectWithLexical_RejectsInsufficientDepth(t *testing.T) {
	_, err := (&Engine{}).CollectWithLexical(context.Background(), Prepared{}, Depths{Dense: 5, Lexical: 5}, LexicalArms{Depth: 4})
	if err == nil {
		t.Fatal("expected an error for insufficient lexical depth")
	}
}

func TestLexicalPrefix_CopiesOnlyLexicalFields(t *testing.T) {
	source := []Candidate{{ChunkID: 7, BM25: 2.5, Rank: 1, Content: "hydrated", Cosine: .7}, {ChunkID: 8, Rank: 2}}
	got := lexicalPrefix(source, 1)
	want := []Candidate{{ChunkID: 7, BM25: 2.5, Rank: 1}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	got[0].ChunkID = 9
	if source[0].ChunkID != 7 {
		t.Fatal("source was modified")
	}
}
