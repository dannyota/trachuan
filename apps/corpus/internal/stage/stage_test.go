package stage

import (
	"errors"
	"fmt"
	"testing"
)

func TestRetryable_WrappedError(t *testing.T) {
	cause := errors.New("temporary failure")
	err := fmt.Errorf("process item: %w", Retryable(cause))
	if !errors.Is(err, cause) {
		t.Fatal("retryable error lost its cause")
	}
	var retryable *RetryableError
	if !errors.As(err, &retryable) {
		t.Fatal("retryable error lost its classification")
	}
}
