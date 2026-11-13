package retrieval

import (
	"context"
	"errors"
)

// LexicalArms holds the lexical candidates and the depth used to collect them.
type LexicalArms struct {
	Primary    []Candidate
	NonCurrent []Candidate
	Depth      int
}

// CollectWithLexical collects dense candidates using previously collected lexical arms.
func (e *Engine) CollectWithLexical(ctx context.Context, p Prepared, depths Depths, lexical LexicalArms) (Arms, error) {
	if lexical.Depth < depths.Lexical {
		return Arms{}, errors.New("insufficient lexical source depth")
	}
	return e.collectWithLexical(ctx, p, depths, false, &lexical)
}

func lexicalPrefix(source []Candidate, depth int) []Candidate {
	if len(source) == 0 {
		return nil
	}
	out := make([]Candidate, min(len(source), depth))
	for i := range out {
		out[i] = Candidate{ChunkID: source[i].ChunkID, BM25: source[i].BM25, Rank: source[i].Rank}
	}
	return out
}
