package chinhphu

import (
	"errors"
	"fmt"
)

var errInvalidPayload = errors.New("invalid payload")

// Failure classifies a source or transport failure without response contents.
type Failure struct {
	Origin, Cause, Endpoint string
	HTTPStatus              int
	Err                     error
}

// Error describes the failure without source payloads.
func (f *Failure) Error() string {
	return fmt.Sprintf("chinhphu %s: %s (status %d)", f.Endpoint, f.Cause, f.HTTPStatus)
}

// Unwrap exposes the underlying classified error.
func (f *Failure) Unwrap() error { return f.Err }
