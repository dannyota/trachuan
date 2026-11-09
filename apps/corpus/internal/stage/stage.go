package stage

import (
	"context"
	"time"

	"github.com/dannyota/traquydinh/apps/corpus/internal/artifact"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Env provides the resources a stage uses to process a run.
type Env struct {
	DB        *pgxpool.Pool
	Artifacts artifact.Store
	Now       func() time.Time
}

// Output describes an item's processing result.
type Output struct {
	Artifact       *artifact.Result
	State, Message string
	Gaps           []Gap
}

// Resolution describes the run-specific rows and gaps for an artifact.
type Resolution struct {
	RunRows func(context.Context, pgx.Tx) error
	Gaps    []Gap
}

// Stage plans, processes, and resolves items for a release.
type Stage interface {
	Name() string
	Recipe(Snapshot) string
	Plan(context.Context, Env, Run) ([]Item, error)
	Process(context.Context, Env, Run, Item) (Output, error)
	Rows(context.Context, artifact.Result) (func(context.Context, pgx.Tx, int64) error, error)
	Resolve(context.Context, Env, Run, Item, artifact.Result) (Resolution, error)
}

// RetryableError marks an item failure that can be retried.
type RetryableError struct{ Err error }

// Error returns the underlying error message.
func (e *RetryableError) Error() string { return e.Err.Error() }

// Unwrap returns the underlying error.
func (e *RetryableError) Unwrap() error { return e.Err }

// Retryable marks an item error for another activity attempt.
func Retryable(err error) error { return &RetryableError{Err: err} }
