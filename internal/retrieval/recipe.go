package retrieval

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// ANNRecipe defines approximate search settings.
type ANNRecipe struct {
	PoolMin           int    `json:"pool_min"`
	PoolMultiplier    int    `json:"pool_multiplier"`
	EFMax             int    `json:"ef_max"`
	IterativeScan     string `json:"iterative_scan"`
	PlanCacheMode     string `json:"plan_cache_mode"`
	MaxScanTuples     int    `json:"max_scan_tuples"`
	ScanMemMultiplier int    `json:"scan_mem_multiplier"`
	Fallback          string `json:"fallback"`
}

// RRFRecipe defines arm fusion weights.
type RRFRecipe struct {
	K                   int     `json:"k"`
	DenseWeight         float64 `json:"dense_weight"`
	LexicalWeight       float64 `json:"lexical_weight"`
	RoutedLexicalWeight float64 `json:"routed_lexical_weight"`
}

// AbstentionRecipe defines the calibrated confidence gate.
type AbstentionRecipe struct {
	Statistic         string   `json:"statistic"`
	Threshold         *float64 `json:"threshold,omitempty"`
	Status            string   `json:"status"`
	CalibrationSHA256 string   `json:"calibration_sha256,omitempty"`
}

// Recipe is the serialized retrieval recipe.
type Recipe struct {
	Version                 int              `json:"version"`
	DenseDepth              int              `json:"dense_depth"`
	LexicalDepth            int              `json:"lexical_depth"`
	ANN                     ANNRecipe        `json:"ann"`
	RRF                     RRFRecipe        `json:"rrf"`
	Ties                    string           `json:"ties"`
	Routing                 string           `json:"routing"`
	Group                   string           `json:"group"`
	Reducer                 string           `json:"reducer"`
	HydrateMultiplier       int              `json:"hydrate_multiplier"`
	DocumentCap             int              `json:"document_cap"`
	NonCurrentTail          int              `json:"non_current_tail"`
	Backfill                string           `json:"backfill"`
	FamilyCollapse          string           `json:"family_collapse"`
	SiblingMerge            string           `json:"sibling_merge,omitempty"`
	ContextScope            string           `json:"context_scope"`
	Validity                string           `json:"validity"`
	DefaultInForceOnly      bool             `json:"default_in_force_only"`
	QueryGate               string           `json:"query_gate"`
	Evidence                string           `json:"evidence"`
	EvidenceRuneCap         int              `json:"evidence_rune_cap"`
	RelationCap             int              `json:"relation_cap"`
	RelationEvidenceRuneCap int              `json:"relation_evidence_rune_cap"`
	Abstention              AbstentionRecipe `json:"abstention"`
}

// Recipe2 returns the uncalibrated release-1 retrieval recipe.
func Recipe2() Recipe {
	return Recipe{2, 50, 50, ANNRecipe{400, 24, 1000, "strict_order", "force_custom_plan", 20000, 1, "exact"}, RRFRecipe{60, 1, .5, 1}, "score-id", "vn-number/1", "nearest-khoan", "max", 8, 5, 2, "demoted-order", "off-unverified-alignment", "", "number-routed/1", "document-dates/3", true, "vn-marks/1", "source-spans/1", 14000, 20, 1000, AbstentionRecipe{"first-group-best-member-cosine", nil, "provisional-old-product", ""}}
}

// Recipe3 returns the uncalibrated recipe with sibling clause merging.
func Recipe3() Recipe {
	r := Recipe2()
	r.Version = 3
	r.SiblingMerge = "article-clauses/1"
	return r
}

func supportedRecipe(r Recipe) bool { return r == Recipe2() || r == Recipe3() }

// JSON returns compact fixed-field JSON.
func (r Recipe) JSON() ([]byte, error) { return json.Marshal(r) }

// Hash returns the SHA-256 of the recipe JSON.
func (r Recipe) Hash() string {
	b, e := r.JSON()
	if e != nil {
		return ""
	}
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}
