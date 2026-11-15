package embed

import (
	"context"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/stage/chunk"
)

func TestBuildUncached_RejectsLongInput(t *testing.T) {
	r := recipe()
	f := &fake{counts: map[string]int{"\nlong": 5}}
	idx, vectors, err := BuildUncached(context.Background(), f.client(r), r, chunk.File{DocumentKey: "fw:example:v1", Chunks: []chunk.Chunk{{Ordinal: 0, Content: "long"}}}, strings.Repeat("a", 64))
	if err != nil || len(vectors) != 0 || len(idx.Rejected) != 1 || idx.Rejected[0].Kind != "embedding_input_too_long" {
		t.Fatalf("unexpected result: %+v, %v", idx, err)
	}
}
