package evaluation

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"

	"danny.vn/trachuan/internal/retrieval"
)

const cacheLimit = int64(2 << 30)

// CachedCandidate is the non-text identity and score of an arm entry.
type CachedCandidate struct {
	ChunkID  int64              `json:"chunk_id"`
	Identity retrieval.Identity `json:"identity"`
	Rank     int                `json:"rank"`
	Score    float64            `json:"score"`
}

// CachedGroup identifies a recipe-shaped group.
type CachedGroup struct {
	Arms       retrieval.ArmMode  `json:"arms"`
	Limit      int                `json:"limit"`
	Identity   retrieval.Identity `json:"identity"`
	Score      float64            `json:"score"`
	BestCosine float64            `json:"best_cosine"`
}

// ArmRow records candidates for a case and retrieval mode.
type ArmRow struct {
	Version         int               `json:"version"`
	CaseID          string            `json:"case_id"`
	Cohort          string            `json:"cohort"`
	ReleaseHash     string            `json:"release_hash"`
	RecipeHashes    map[string]string `json:"recipe_hashes"`
	EvaluationDate  retrieval.Date    `json:"evaluation_date"`
	RequestAsOf     *retrieval.Date   `json:"request_as_of"`
	Mode            retrieval.Mode    `json:"mode"`
	Depths          retrieval.Depths  `json:"depths"`
	Route           bool              `json:"route"`
	EligibilityHash string            `json:"eligibility_hash"`
	Dense           []CachedCandidate `json:"dense"`
	Lexical         []CachedCandidate `json:"lexical"`
	RawANN          []CachedCandidate `json:"raw_ann"`
	Fallback        bool              `json:"fallback"`
	Groups          []CachedGroup     `json:"groups"`
	Diagnostics     map[string]any    `json:"diagnostics"`
}

// QueryIndex identifies an authored input's vector without storing its text.
type QueryIndex struct {
	CaseID      string `json:"case_id"`
	Cohort      string `json:"cohort"`
	Row         *int   `json:"row,omitempty"`
	Count       int    `json:"count,omitempty"`
	QuerySHA256 string `json:"query_sha256,omitempty"`
	RecipeHash  string `json:"query_recipe_hash,omitempty"`
	Reason      string `json:"reason,omitempty"`
}

// Cache contains the arm and query-vector artifacts of a run.
type Cache struct {
	Arms    []ArmRow
	Index   []QueryIndex
	Vectors [][]float32
}

func cachedCandidates(cs []retrieval.Candidate, lexical bool) []CachedCandidate {
	out := make([]CachedCandidate, 0, len(cs))
	for _, c := range cs {
		score := c.Cosine
		if lexical {
			score = c.BM25
		}
		out = append(out, CachedCandidate{c.ChunkID, c.Identity, c.Rank, score})
	}
	return out
}

// WriteFiles writes the arm lists and little-endian query vectors.
func (c Cache) WriteFiles(dir string) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create cache directory: %w", err)
	}
	var size int64
	for _, row := range c.Vectors {
		size += int64(len(row)) * 4
		if size > cacheLimit {
			return fmt.Errorf("evaluation cache exceeds 2 GiB")
		}
	}
	if err := writeJSONLines(filepath.Join(dir, "arms.jsonl"), c.Arms); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(dir, "query-index.json"), c.Index); err != nil {
		return err
	}
	f, err := os.Create(filepath.Join(dir, "queries.f32"))
	if err != nil {
		return fmt.Errorf("create query vectors: %w", err)
	}
	for _, row := range c.Vectors {
		for _, v := range row {
			if err = binary.Write(f, binary.LittleEndian, math.Float32bits(v)); err != nil {
				_ = f.Close()
				return fmt.Errorf("write query vectors: %w", err)
			}
		}
	}
	if err = f.Close(); err != nil {
		return fmt.Errorf("close query vectors: %w", err)
	}
	return checkArtifactSize(dir)
}
func checkArtifactSize(dir string) error {
	var size int64
	for _, name := range []string{"arms.jsonl", "query-index.json", "queries.f32", "report.json", "cases.jsonl"} {
		info, err := os.Stat(filepath.Join(dir, name))
		if err == nil {
			size += info.Size()
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("stat evaluation file: %w", err)
		}
	}
	if size > cacheLimit {
		return fmt.Errorf("evaluation cache exceeds 2 GiB")
	}
	return nil
}

type timingSummary struct {
	N   int      `json:"n"`
	P50 *float64 `json:"p50"`
	P95 *float64 `json:"p95"`
	P99 *float64 `json:"p99"`
}

func summarizeTiming(values []float64) timingSummary {
	sort.Float64s(values)
	return timingSummary{len(values), Quantile(values, .5), Quantile(values, .95), Quantile(values, .99)}
}

func cacheKey(c Cache, o Options, hashes FixtureHashes) string {
	var policies any
	if o.HistoricalPolicy != nil || o.NotYetEligible || (o.NonCurrentTail != nil && *o.NonCurrentTail > 0) {
		policies = o.ValidityOverrides
	}
	value := struct {
		Version  string
		Dump     string
		Fixtures FixtureHashes
		Date     retrieval.Date
		Modes    []retrieval.Mode
		Recipes  map[string]string
		Software map[string]string
		Index    []QueryIndex
		Vectors  [][]float32
		Policies any `json:",omitempty"`
	}{"agent-eval/2", o.DumpSHA256, hashes, o.EvaluationDate, o.Modes, o.Recipes, o.Software, c.Index, c.Vectors, policies}
	b, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
func eligibilityHash(date retrieval.Date) string {
	sum := sha256.Sum256([]byte("current/" + date.String()))
	return hex.EncodeToString(sum[:])
}

func withReleaseRecipes(o Options, rel retrieval.Release, modes []retrieval.Mode) Options {
	recipes := make(map[string]string, len(o.Recipes)+4)
	for k, v := range o.Recipes {
		recipes[k] = v
	}
	recipes["retrieval"] = rel.RetrievalRecipe.Hash()
	recipes["lexical"] = rel.LexicalRecipe.Hash()
	recipes["query"] = rel.DenseRecipe.QueryHash
	recipes["document"] = rel.DenseRecipe.DocumentHash
	sum := sha256.Sum256(rel.ChunkRecipe)
	recipes["chunk"] = hex.EncodeToString(sum[:])
	o.Recipes = recipes
	o.Modes = append([]retrieval.Mode(nil), modes...)
	return o
}

func caseMappings(entries map[int]MapEntry, count int) [][]Alternative {
	out := make([][]Alternative, count)
	for i := range out {
		out[i] = entries[i].Alternatives
	}
	return out
}
func matchedCount(ranks []int) int {
	n := 0
	for _, rank := range ranks {
		if rank > 0 {
			n++
		}
	}
	return n
}
