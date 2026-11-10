//go:build !ort

package tokenizer

import (
	"errors"
	"testing"
)

func TestStubs_ReturnErrNativeUnavailable(t *testing.T) {
	if _, err := Open("unused"); !errors.Is(err, ErrNativeUnavailable) {
		t.Fatalf("Open error = %v", err)
	}
}
