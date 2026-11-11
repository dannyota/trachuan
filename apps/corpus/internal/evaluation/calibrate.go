package evaluation

import (
	"context"
	"fmt"
	"math"
	"sort"

	"danny.vn/trachuan/internal/retrieval"
)

// Engine is the retrieval boundary used by evaluation.
type Engine interface {
	Prepare(context.Context, retrieval.Request) (retrieval.Prepared, error)
	PrepareLegacy(context.Context, retrieval.Request) (retrieval.Prepared, error)
	Collect(context.Context, retrieval.Prepared, retrieval.Depths) (retrieval.Arms, error)
	Present(context.Context, retrieval.Response, retrieval.Detail) (retrieval.SearchResult, error)
	Release() retrieval.Release
}

// CalibrationCandidate records decisions at one threshold.
type CalibrationCandidate struct {
	Threshold float64 `json:"threshold"`
	TP        int     `json:"tp"`
	FP        int     `json:"fp"`
	TN        int     `json:"tn"`
	FN        int     `json:"fn"`
}

// Calibration records the fitted threshold and its evidence.
type Calibration struct {
	Status         string                   `json:"status"`
	Threshold      float64                  `json:"threshold"`
	Success        bool                     `json:"success"`
	Scores         []CalibrationObservation `json:"scores"`
	Candidates     []CalibrationCandidate   `json:"candidates"`
	FixtureHashes  FixtureHashes            `json:"fixture_hashes"`
	RecipeHash     string                   `json:"recipe_hash"`
	EvaluationDate retrieval.Date           `json:"evaluation_date"`
}

// CalibrationObservation records a case's pre-threshold score.
type CalibrationObservation struct {
	CaseID   string   `json:"case_id"`
	Negative bool     `json:"negative"`
	Cosine   *float64 `json:"cosine"`
}
type calibrationScore struct {
	value    float64
	negative bool
	empty    bool
}

func calibrationThreshold(scores []calibrationScore) (float64, []CalibrationCandidate) {
	values := []float64{}
	for _, s := range scores {
		if !s.empty {
			values = append(values, s.value)
		}
	}
	sort.Float64s(values)
	thresholds := []float64{-1}
	for i := 1; i < len(values); i++ {
		a, b := values[i-1], values[i]
		if a == b {
			continue
		}
		m := a + (b-a)/2
		if m <= a {
			m = b
		}
		thresholds = append(thresholds, m)
	}
	thresholds = append(thresholds, 2)
	best, bestTN := 2.0, 0
	candidates := make([]CalibrationCandidate, 0, len(thresholds))
	for _, threshold := range thresholds {
		c := CalibrationCandidate{Threshold: threshold}
		for _, s := range scores {
			abstain := s.empty || s.value < threshold
			switch {
			case s.negative && abstain:
				c.TP++
			case s.negative:
				c.FN++
			case abstain:
				c.FP++
			default:
				c.TN++
			}
		}
		candidates = append(candidates, c)
		if c.FN == 0 && c.TN > bestTN {
			best, bestTN = threshold, c.TN
		}
	}
	return best, candidates
}

// Calibrate fits both product cohorts using exact retrieval.
func Calibrate(ctx context.Context, engine Engine, suite Suite) (Calibration, error) {
	if err := validateSuite(suite); err != nil {
		return Calibration{}, err
	}
	recipe := engine.Release().RetrievalRecipe
	recipe.Abstention.Threshold = nil
	recipe.Abstention.CalibrationSHA256 = ""
	out := Calibration{Status: "provisional-old-product", FixtureHashes: suite.Hashes, RecipeHash: recipe.Hash()}
	if len(suite.Development) > 0 {
		out.Status = "old-product+development"
	}
	rejects := rejectionSet(suite)
	scores := []calibrationScore{}
	for _, c := range append(append([]Case{}, suite.Legacy...), suite.Development...) {
		if rejects[c.ID] {
			continue
		}
		force := true
		p, err := engine.Prepare(ctx, retrieval.Request{Query: c.Question, Limit: 8, Mode: "exact", Arms: "hybrid", InForceOnly: &force})
		if err != nil {
			return Calibration{}, fmt.Errorf("prepare case %s: %w", c.ID, err)
		}
		observation := CalibrationObservation{CaseID: c.ID, Negative: c.ExpectAbstain}
		score := calibrationScore{negative: c.ExpectAbstain, empty: true}
		if p.Validation.Accepted {
			arms, err := engine.Collect(ctx, p, retrieval.Depths{Dense: recipe.DenseDepth, Lexical: recipe.LexicalDepth})
			if err != nil {
				return Calibration{}, fmt.Errorf("collect case %s: %w", c.ID, err)
			}
			out.EvaluationDate = arms.Date
			ranking, err := retrieval.Rank(arms, recipe, 8, "hybrid")
			if err != nil {
				return Calibration{}, fmt.Errorf("rank case %s: %w", c.ID, err)
			}
			if ranking.FirstBestCosine != nil {
				v := *ranking.FirstBestCosine
				if math.IsNaN(v) || math.IsInf(v, 0) {
					return Calibration{}, fmt.Errorf("invalid cosine for case %s", c.ID)
				}
				v = max(-1, min(1, v))
				observation.Cosine = &v
				score.value = v
				score.empty = false
			}
		}
		scores = append(scores, score)
		out.Scores = append(out.Scores, observation)
	}
	out.Threshold, out.Candidates = calibrationThreshold(scores)
	out.Success = out.Threshold != 2
	return out, nil
}
