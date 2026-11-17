package evaluate

import (
	"context"
	"encoding/json"
	"fmt"

	"danny.vn/trachuan/internal/frameworks/build"
	"danny.vn/trachuan/internal/frameworks/serve"
	"github.com/jackc/pgx/v5"
)

type Engine interface {
	build.CalibrationEngine
	Search(context.Context, serve.Request, serve.Caller) (serve.Response, error)
	Read(context.Context, serve.ReadRequest, serve.Caller) (serve.ReadResponse, error)
	Mappings(context.Context, serve.MappingsRequest, serve.Caller) (serve.MappingsResponse, error)
	CorpusStatus(context.Context, serve.StatusRequest, serve.Caller) (serve.StatusResponse, error)
	QualityGaps(context.Context, serve.GapsRequest, serve.Caller) (serve.GapsResponse, error)
}
type Opener func(context.Context, *pgx.Conn, []byte) (Engine, func(), error)
type Restore func(context.Context, build.DatabaseRequest, string, bool, func(context.Context, *pgx.Conn) error) error

type EvaluationRequest = build.EvaluationRequest
type EvaluationResult = build.EvaluationResult
type EvaluationEngine = Engine
type GoldenCase = build.GoldenCase
type Control = build.Control
type CaseResult = build.CaseResult
type CaseOutcome = build.CaseOutcome
type NearHit = build.NearHit
type CohortMetrics = build.CohortMetrics
type GapTotal = build.GapTotal
type InvariantResult = build.InvariantResult
type InvariantCaller = build.InvariantCaller
type InvariantClass = build.InvariantClass
type InvariantFailure = build.InvariantFailure

type caseFailure struct {
	id, operation string
	cause         error
}

func (e *caseFailure) Error() string { return fmt.Sprintf("case %q: %s", e.id, e.operation) }
func (e *caseFailure) Unwrap() error { return e.cause }

type hiddenError struct{ cause error }

func (e *hiddenError) Error() string { return fmt.Sprintf("%T", e.cause) }
func (e *hiddenError) Unwrap() error { return e.cause }
func hide(operation string, err error) error {
	return fmt.Errorf("%s: %w", operation, &hiddenError{err})
}
func equalJSON(a, b []byte) bool {
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return false
	}
	return build.RecipeSHA256(x) == build.RecipeSHA256(y)
}
func cohortCaller(cohort string) string {
	if cohort == "public" {
		return "free-only"
	}
	return "full"
}
