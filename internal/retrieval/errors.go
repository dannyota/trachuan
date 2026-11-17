package retrieval

import (
	"errors"

	"danny.vn/trachuan/internal/embedding"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/puddle/v2"
)

type failure string

func (f failure) Error() string { return string(f) }

const (
	ErrInvalidRequest failure = "invalid_request"
	ErrReleaseExpired failure = "release_expired"
	ErrNotFound       failure = "not_found"
	ErrEmbedderBusy   failure = "embedder busy"
	ErrEmbedder       failure = "embedder failed"
	ErrReleaseGone    failure = "release gone"
	ErrUnavailable    failure = "database unavailable"
)

type classifiedFailure struct {
	message string
	kind    error
	code    string
}

func (f classifiedFailure) Error() string { return f.message }
func (f classifiedFailure) Unwrap() error { return f.kind }

func (f classifiedFailure) As(target any) bool {
	p, ok := target.(**pgconn.PgError)
	if !ok || f.code == "" {
		return false
	}
	*p = &pgconn.PgError{Code: f.code}
	return true
}

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
	var code string
	if errors.As(cause, &database) {
		code = database.Code
	}
	return classifiedFailure{message: message, kind: kind, code: code}
}
