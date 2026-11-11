package retrieval

import (
	"errors"

	"danny.vn/trachuan/internal/embedding"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/puddle/v2"
)

type failure string

// Error returns the fixed failure category.
func (f failure) Error() string { return string(f) }

const (
	// ErrEmbedderBusy means the query embedder refused admission.
	ErrEmbedderBusy failure = "embedder busy"
	// ErrEmbedder means the query embedder failed.
	ErrEmbedder failure = "embedder failed"
	// ErrReleaseGone means the corpus database or its pool is gone.
	ErrReleaseGone failure = "release gone"
	// ErrUnavailable means the corpus database failed for another reason.
	ErrUnavailable failure = "database unavailable"
)

// classifiedFailure retains only a fixed message and a safe classification.
// The original cause can contain query text, vectors or connection details.
type classifiedFailure struct {
	message string
	kind    error
}

func (f classifiedFailure) Error() string { return f.message }
func (f classifiedFailure) Unwrap() error { return f.kind }

func embedderFailure(message string, cause error) error {
	kind := ErrEmbedder
	var service *embedding.Error
	if errors.As(cause, &service) && (service.Status == 429 || service.Code == "busy") {
		kind = ErrEmbedderBusy
	}
	return classifiedFailure{message: message, kind: kind}
}

func databaseFailure(message string, cause error) error {
	kind := ErrUnavailable
	var database *pgconn.PgError
	if errors.Is(cause, ErrReleaseGone) || errors.Is(cause, puddle.ErrClosedPool) ||
		(errors.As(cause, &database) && (database.Code == "3D000" || database.Code == "57P01" || database.Code == "57P04")) {
		kind = ErrReleaseGone
	}
	return classifiedFailure{message: message, kind: kind}
}
