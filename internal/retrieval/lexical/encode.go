package lexical

import (
	"fmt"
	"math"
	"sort"
)

// Entry is a one-based sparse vector coordinate and its weight.
type Entry struct {
	Index int32
	Value float32
}

// Vector contains entries sorted by index.
type Vector []Entry

// Document encodes a chunk with BM25 weights from corpus statistics.
func Document(text string, stats Stats, recipe Recipe) (Vector, error) {
	tokens, err := Tokenize(text, recipe)
	if err != nil {
		return nil, fmt.Errorf("tokenize document: %w", err)
	}
	if len(tokens) == 0 {
		return Vector{}, nil
	}
	tf := make(map[string]int)
	for _, token := range tokens {
		tf[token]++
	}
	terms := make([]string, 0, len(tf))
	for token := range tf {
		terms = append(terms, token)
	}
	sort.Strings(terms)
	weights := make(map[int32]float64)
	for _, token := range terms {
		df := stats.DF[token]
		if stats.N <= 0 || df <= 0 || df > stats.N || stats.AvgDL <= 0 || math.IsNaN(stats.AvgDL) || math.IsInf(stats.AvgDL, 0) {
			return nil, fmt.Errorf("invalid lexical statistics")
		}
		freq := float64(tf[token])
		idf := math.Log(1 + (float64(stats.N-df)+0.5)/(float64(df)+0.5))
		weights[Dimension(token, recipe)] += idf * freq * (recipe.K1 + 1) / (freq + recipe.K1*(1-recipe.B+recipe.B*float64(len(tokens))/stats.AvgDL))
	}
	if len(weights) > 16000 {
		return nil, fmt.Errorf("lexical vector exceeds 16000 nonzeros")
	}
	result := make(Vector, 0, len(weights))
	for index, weight := range weights {
		value := float32(weight)
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) || value <= 0 {
			return nil, fmt.Errorf("invalid lexical weight")
		}
		result = append(result, Entry{index, value})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Index < result[j].Index })
	return result, nil
}

// Query encodes the presence of each hashed dimension.
func Query(text string, recipe Recipe) (Vector, error) {
	tokens, err := Tokenize(text, recipe)
	if err != nil {
		return nil, fmt.Errorf("tokenize query: %w", err)
	}
	dims := make(map[int32]struct{})
	for _, token := range tokens {
		dims[Dimension(token, recipe)] = struct{}{}
	}
	result := make(Vector, 0, len(dims))
	for d := range dims {
		result = append(result, Entry{d, 1})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Index < result[j].Index })
	return result, nil
}
