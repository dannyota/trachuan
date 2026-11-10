package lexical

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
)

// Stats contains corpus-wide document frequencies and lengths.
type Stats struct {
	N, TotalTokens int64
	AvgDL          float64
	DF             map[string]int64
}

// TextIterator streams the inserted chunks in order.
type TextIterator interface {
	Next(context.Context) (string, bool, error)
}

// Train collects statistics across all chunks, including empty chunks.
func Train(ctx context.Context, texts TextIterator, recipe Recipe) (Stats, error) {
	if err := recipe.validate(); err != nil {
		return Stats{}, err
	}
	s := Stats{DF: make(map[string]int64)}
	for {
		if err := ctx.Err(); err != nil {
			return Stats{}, fmt.Errorf("train lexical statistics: %w", err)
		}
		text, ok, err := texts.Next(ctx)
		if err != nil {
			return Stats{}, fmt.Errorf("read lexical text: %w", err)
		}
		if !ok {
			break
		}
		tokens, err := Tokenize(text, recipe)
		if err != nil {
			return Stats{}, fmt.Errorf("tokenize lexical text: %w", err)
		}
		s.N++
		s.TotalTokens += int64(len(tokens))
		seen := make(map[string]struct{}, len(tokens))
		for _, token := range tokens {
			seen[token] = struct{}{}
		}
		for token := range seen {
			s.DF[token]++
		}
	}
	s.AvgDL = 1
	if s.N > 0 && s.TotalTokens > 0 {
		s.AvgDL = float64(s.TotalTokens) / float64(s.N)
	}
	return s, nil
}

// NNZSummary gives nearest-rank nonzero counts.
type NNZSummary struct {
	P50 int `json:"p50"`
	P95 int `json:"p95"`
	Max int `json:"max"`
}

// StatsSummary is the persisted lexical_stats object.
type StatsSummary struct {
	Version            int        `json:"version"`
	N                  int64      `json:"N"`
	TotalTokens        int64      `json:"total_tokens"`
	AvgDL              float64    `json:"avgdl"`
	VocabularySHA256   string     `json:"vocabulary_sha256"`
	DFSHA256           string     `json:"df_sha256"`
	DistinctTerms      int        `json:"distinct_terms"`
	OccupiedDimensions int        `json:"occupied_dimensions"`
	CollisionTerms     int        `json:"collision_terms"`
	NNZ                NNZSummary `json:"nnz"`
}

// Summary computes deterministic corpus fingerprints and nonzero counts.
func (s Stats) Summary(recipe Recipe, nnz []int) StatsSummary {
	terms := make([]string, 0, len(s.DF))
	for token := range s.DF {
		terms = append(terms, token)
	}
	sort.Strings(terms)
	vocab := make([][2]any, 0, len(terms))
	dfs := make([][2]any, 0, len(terms))
	occupied := make(map[int32]struct{})
	for _, token := range terms {
		d := Dimension(token, recipe)
		vocab = append(vocab, [2]any{token, d})
		dfs = append(dfs, [2]any{token, s.DF[token]})
		occupied[d] = struct{}{}
	}
	vb, _ := json.Marshal(vocab)
	db, _ := json.Marshal(dfs)
	vh := sha256.Sum256(vb)
	dh := sha256.Sum256(db)
	counts := append([]int(nil), nnz...)
	sort.Ints(counts)
	var sizes NNZSummary
	if len(counts) > 0 {
		sizes = NNZSummary{counts[int(math.Ceil(.5*float64(len(counts))))-1], counts[int(math.Ceil(.95*float64(len(counts))))-1], counts[len(counts)-1]}
	}
	return StatsSummary{recipe.Version, s.N, s.TotalTokens, s.AvgDL, hex.EncodeToString(vh[:]), hex.EncodeToString(dh[:]), len(terms), len(occupied), len(terms) - len(occupied), sizes}
}
