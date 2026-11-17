package evaluation

import (
	"context"
	"fmt"
	"math"

	"danny.vn/trachuan/internal/retrieval"
	"danny.vn/trachuan/internal/retrieval/abstention"
)

type Engine interface {
	Prepare(context.Context, retrieval.Request) (retrieval.Prepared, error)
	PrepareLegacy(context.Context, retrieval.Request) (retrieval.Prepared, error)
	Collect(context.Context, retrieval.Prepared, retrieval.Depths) (retrieval.Arms, error)
	Present(context.Context, retrieval.Response, retrieval.Detail) (retrieval.SearchResult, error)
	Release() retrieval.Release
}

type CalibrationCandidate = abstention.Candidate

type Calibration struct {
	Status                 string                   `json:"status"`
	Threshold              float64                  `json:"threshold"`
	Success                bool                     `json:"success"`
	PositiveAnswerRate     *float64                 `json:"positive_answer_rate"`
	NegativeAbstentionRate *float64                 `json:"negative_abstention_rate"`
	BalancedAccuracy       *float64                 `json:"balanced_accuracy"`
	Scores                 []CalibrationObservation `json:"scores"`
	Candidates             []CalibrationCandidate   `json:"candidates"`
	FixtureHashes          FixtureHashes            `json:"fixture_hashes"`
	RecipeHash             string                   `json:"recipe_hash"`
	EvaluationDate         retrieval.Date           `json:"evaluation_date"`
}

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
	shared := make([]abstention.Score, 0, len(scores))
	for _, score := range scores {
		shared = append(shared, abstention.Score{Value: score.value, Negative: score.negative, Empty: score.empty})
	}
	return abstention.Threshold(shared)
}

func Calibrate(ctx context.Context, engine Engine, suite Suite) (Calibration, error) {
	return calibrate(ctx, engine, suite, caseConcurrency)
}

func calibrate(ctx context.Context, engine Engine, suite Suite, limit int) (Calibration, error) {
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
	var cases []Case
	for _, c := range append(append([]Case{}, suite.Legacy...), suite.Development...) {
		if !rejects[c.ID] {
			cases = append(cases, c)
		}
	}
	depths := retrieval.Depths{Dense: recipe.DenseDepth, Lexical: recipe.LexicalDepth}
	pool := newOrderedPool(ctx, len(cases), limit, func(ctx context.Context, i int) calibrationPrefetch {
		return prefetchCalibration(ctx, engine, cases[i], depths)
	})
	results := make([]calibrationPrefetch, len(cases))
	for i := range cases {
		results[i] = pool.take(i)
	}
	pool.stop()
	for i := range cases {
		if sqlState(results[i].collectErr) == "57014" {
			results[i].arms, results[i].collectErr = engine.Collect(ctx, results[i].prepared, depths)
		}
	}
	for i, c := range cases {
		result := results[i]
		p, err := result.prepared, result.prepareErr
		if err != nil {
			return Calibration{}, fmt.Errorf("prepare case %s: %w", c.ID, OfflineError(err))
		}
		observation := CalibrationObservation{CaseID: c.ID, Negative: c.ExpectAbstain}
		score := calibrationScore{negative: c.ExpectAbstain, empty: true}
		if p.Validation.Accepted {
			arms, err := result.arms, result.collectErr
			if err != nil {
				return Calibration{}, fmt.Errorf("collect case %s: %w", c.ID, OfflineError(err))
			}
			out.EvaluationDate = arms.Date
			ranking, err := retrieval.Rank(arms, recipe, 8, "hybrid")
			if err != nil {
				return Calibration{}, fmt.Errorf("rank case %s: %w", c.ID, OfflineError(err))
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
	for _, c := range out.Candidates {
		if c.Threshold != out.Threshold {
			continue
		}
		out.PositiveAnswerRate = ratio(c.TN, c.TN+c.FP)
		out.NegativeAbstentionRate = ratio(c.TP, c.TP+c.FN)
		out.Success = out.PositiveAnswerRate != nil && out.NegativeAbstentionRate != nil
		if out.Success {
			v := (*out.PositiveAnswerRate + *out.NegativeAbstentionRate) / 2
			out.BalancedAccuracy = &v
		}
		break
	}
	return out, nil
}
