package build

import (
	"context"
)

type EvaluationRequest struct {
	DatabaseRequest
	Cases             []GoldenCase
	CaseFileSHA256    string
	Calibration       *CalibrationRecord
	CalibrationSHA256 string
	RecipeHashes      map[string]string
	InputHashes       map[string]string
	RetrievalRecipe   map[string]any
	Controls          []Control
	ProbeTargets      []string
}

type Evaluator interface {
	Evaluate(context.Context, EvaluationRequest, func(EvaluationResult) error) error
}

type EvaluationResult struct {
	Restored              bool                     `json:"restored"`
	Cases                 []CaseResult             `json:"cases"`
	Cohorts               map[string]CohortMetrics `json:"cohorts"`
	Invariant             InvariantResult          `json:"invariant"`
	Gaps                  []GapTotal               `json:"gap_totals"`
	RetrievalRecipeStatus string                   `json:"retrieval_recipe_status"`
}

type GapTotal struct {
	Kind   string `json:"kind"`
	Class  string `json:"class"`
	Blocks bool   `json:"blocks"`
	Count  int    `json:"count"`
}

type CaseResult struct {
	Cohorts      map[string]map[string]CaseOutcome `json:"cohorts"`
	ID           string                            `json:"id"`
	Adjudication string                            `json:"adjudication"`
	Target       string                            `json:"target"`
	Code         string                            `json:"code"`
	Version      string                            `json:"version"`
	Access       string                            `json:"access,omitempty"`
	Negative     bool                              `json:"negative"`
	Results      map[string]map[string]CaseOutcome `json:"callers"`
	Filter       map[string]string                 `json:"filter"`
}

type CaseOutcome struct {
	Rank          int       `json:"rank"`
	Miss          bool      `json:"miss"`
	Abstention    string    `json:"abstention"`
	Error         bool      `json:"error"`
	NearHits      []NearHit `json:"near_hits"`
	CurrentHits   int       `json:"current_version_hits"`
	FrameworkHits int       `json:"framework_hits"`
}

type NearHit struct {
	Handle string `json:"handle"`
	Kind   string `json:"kind"`
	Rank   int    `json:"rank"`
}

type Metric struct {
	Status      string   `json:"status"`
	Numerator   float64  `json:"numerator"`
	Denominator int      `json:"denominator"`
	Value       *float64 `json:"value"`
	Noise       string   `json:"noise"`
}

type ModeMetrics struct {
	NearHits               map[string]int `json:"near_hits"`
	Recall                 Metric         `json:"recall_at_8"`
	MRR                    Metric         `json:"mrr_at_8"`
	CurrentVersion         Metric         `json:"current_version_rate"`
	PositiveAnswerRate     Metric         `json:"positive_answer_rate"`
	NegativeAbstentionRate Metric         `json:"negative_abstention_rate"`
	BalancedAccuracy       Metric         `json:"abstention_balanced_accuracy"`
}

type FilterGate struct {
	Pass   bool     `json:"pass"`
	Losses []string `json:"losses"`
	Gains  int      `json:"gains"`
}

type CohortMetrics struct {
	Caller   string                            `json:"caller"`
	Modes    map[string]ModeMetrics            `json:"modes"`
	Versions map[string]map[string]ModeMetrics `json:"versions"`
	Filter   FilterGate                        `json:"filter"`
}

type InvariantFailure struct {
	Cause     FailureCause `json:"cause"`
	Operation string       `json:"operation"`
	Class     string       `json:"class"`
	CaseID    string       `json:"case_id"`
	Mode      string       `json:"mode,omitempty"`
	Page      int          `json:"page"`
	Repeat    int          `json:"repeat"`
}
type InvariantClass struct {
	Status          string `json:"status"`
	PositiveControl bool   `json:"positive_control"`
	Pass            bool   `json:"pass"`
}
type InvariantCaller struct {
	Caller      string                    `json:"caller"`
	Classes     map[string]InvariantClass `json:"classes"`
	Calibration string                    `json:"calibration"`
	Failures    []InvariantFailure        `json:"failures"`
	Pass        bool                      `json:"pass"`
}
type InvariantResult struct {
	Callers       []InvariantCaller `json:"callers"`
	MixedCoverage string            `json:"mixed_coverage"`
	Pass          bool              `json:"pass"`
}
