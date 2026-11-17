package build

import (
	"context"
	"fmt"
	"math"

	"danny.vn/trachuan/internal/embedding"
	"danny.vn/trachuan/internal/retrieval/abstention"
	"github.com/jackc/pgx/v5"
)

type CalibrationEngine interface {
	CalibrationSearch(context.Context, string) (*float64, bool, error)
	CalibrationRecipeHash() string
}

type CalibrationOpener func(context.Context, *pgx.Conn) (CalibrationEngine, func(), error)

type CalibrationObservation struct {
	CaseID   string   `json:"case_id"`
	Negative bool     `json:"negative"`
	Cosine   *float64 `json:"cosine"`
	Lookup   bool     `json:"lookup"`
}

type CalibrationRecord struct {
	Version                   int                      `json:"version"`
	Threshold                 float64                  `json:"threshold"`
	Candidates                []abstention.Candidate   `json:"candidates"`
	Scores                    []CalibrationObservation `json:"scores"`
	LookupCount               int                      `json:"lookup_count"`
	CaseFileSHA256            string                   `json:"case_file_sha256"`
	EngineRecipeSHA256        string                   `json:"engine_recipe_sha256"`
	QueryEmbedderRecipeSHA256 string                   `json:"query_embedder_recipe_sha256"`
	PositiveAnswerRate        float64                  `json:"positive_answer_rate"`
	NegativeAbstentionRate    float64                  `json:"negative_abstention_rate"`
	BalancedAccuracy          float64                  `json:"balanced_accuracy"`
}

func CalibrateCases(ctx context.Context, engine CalibrationEngine, cases []GoldenCase, caseHash string) (CalibrationRecord, error) {
	var public []GoldenCase
	positives, negatives := 0, 0
	for _, c := range cases {
		if c.Adjudication != "accepted" || (!c.Negative && c.Access != "free") {
			continue
		}
		public = append(public, c)
		if c.Negative {
			negatives++
		} else {
			positives++
		}
	}
	if positives == 0 || negatives == 0 {
		return CalibrationRecord{}, fmt.Errorf("missing calibration class")
	}
	if engine == nil {
		return CalibrationRecord{}, fmt.Errorf("calibration engine unavailable")
	}
	r := CalibrationRecord{Version: 1, CaseFileSHA256: caseHash, EngineRecipeSHA256: engine.CalibrationRecipeHash(), QueryEmbedderRecipeSHA256: embedding.QueryRecipe1().Hash()}
	scores := make([]abstention.Score, 0, len(public))
	for _, c := range public {
		if err := ctx.Err(); err != nil {
			return CalibrationRecord{}, &caseFailure{id: c.ID, operation: "calibration canceled", cause: err}
		}
		value, lookup, err := engine.CalibrationSearch(ctx, c.Question)
		if err != nil {
			return CalibrationRecord{}, &caseFailure{id: c.ID, operation: "calibration execution failed", cause: err}
		}
		score := abstention.Score{Negative: c.Negative, Empty: value == nil, FixedAnswer: lookup}
		if value != nil {
			if math.IsNaN(*value) || math.IsInf(*value, 0) {
				return CalibrationRecord{}, &caseFailure{id: c.ID, operation: "non-finite calibration cosine"}
			}
			clamped := max(-1, min(1, *value))
			value = &clamped
			score.Value = clamped
		}
		if lookup {
			r.LookupCount++
			value = nil
		}
		scores = append(scores, score)
		r.Scores = append(r.Scores, CalibrationObservation{CaseID: c.ID, Negative: c.Negative, Cosine: value, Lookup: lookup})
	}
	r.Threshold, r.Candidates = abstention.Threshold(scores)
	for _, c := range r.Candidates {
		if c.Threshold == r.Threshold {
			r.PositiveAnswerRate = float64(c.TN) / float64(positives)
			r.NegativeAbstentionRate = float64(c.TP) / float64(negatives)
			r.BalancedAccuracy = (r.PositiveAnswerRate + r.NegativeAbstentionRate) / 2
			break
		}
	}
	return r, nil
}
