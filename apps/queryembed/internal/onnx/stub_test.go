//go:build !ort

package onnx

import (
	"errors"
	"testing"
)

func TestStubs_ReturnErrNativeUnavailable(t *testing.T) {
	if _, err := OpenBytes([]byte("unused"), QueryOptions("unused")); !errors.Is(err, ErrNativeUnavailable) {
		t.Fatalf("OpenBytes error = %v", err)
	}
	if _, err := Version(); !errors.Is(err, ErrNativeUnavailable) {
		t.Fatalf("Version error = %v", err)
	}
	if got := QueryOptions("library"); got.LibraryPath != "library" || got.IntraOp != 4 || got.InterOp != 1 {
		t.Fatalf("QueryOptions = %+v", got)
	}
}
