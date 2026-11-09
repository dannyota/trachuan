package stage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
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
	Rows(context.Context, Env, artifact.Result) (func(context.Context, pgx.Tx, int64) error, error)
	Resolve(context.Context, Env, Run, Item, artifact.Result) (Resolution, error)
}

// RetryableError marks an item failure that can be retried. When GapKind is
// set and the last attempt still fails, the item becomes that gap instead of
// failed, because the source itself keeps refusing.
type RetryableError struct {
	Err     error
	GapKind string
	Cause   GapCause
}

// Error returns the underlying error message.
func (e *RetryableError) Error() string { return e.Err.Error() }

// Unwrap returns the underlying error.
func (e *RetryableError) Unwrap() error { return e.Err }

// Retryable marks an item error for another activity attempt.
func Retryable(err error) error { return &RetryableError{Err: err} }

// RetryableGap marks a retryable failure that becomes gap kind after the last attempt.
func RetryableGap(err error, kind string) error { return &RetryableError{Err: err, GapKind: kind} }

// ErrSourceData marks errors caused by the source document's content.
var ErrSourceData = errors.New("invalid source data")

// SourceData marks err as caused by the source document's content; the item
// becomes gap invalid_source_data, which the owner may accept.
func SourceData(err error) error { return fmt.Errorf("%w: %w", ErrSourceData, err) }
