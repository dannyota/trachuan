//go:build ort

package onnx

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"

	ort "github.com/microsoft/onnxruntime/go/onnxruntime"
)

// The binding's C environment is process-wide. This sync.Once and its result
// are the sole exception to the rule against package-level mutable state.
// Never shut it down while sessions or concurrent native calls can still exist.
var environment struct {
	once sync.Once
	path string
	err  error
}

func initialize(libraryPath string) error {
	if libraryPath == "" {
		return errors.New("initialize ONNX Runtime: library path is required")
	}
	path, err := filepath.Abs(libraryPath)
	if err != nil {
		return fmt.Errorf("resolve ONNX Runtime library: %w", nativeError{err})
	}
	environment.once.Do(func() {
		environment.path = path
		ort.SetSharedLibraryPath(path)
		environment.err = ort.Init()
	})
	if environment.path != path {
		return errors.New("initialize ONNX Runtime: library path differs from process environment")
	}
	if environment.err != nil {
		return fmt.Errorf("initialize ONNX Runtime: %w", nativeError{environment.err})
	}
	return nil
}

// Version returns the loaded ONNX Runtime library version after Open initializes it.
func Version() (string, error) {
	version, err := ort.GetVersion()
	if err != nil {
		return "", fmt.Errorf("read ONNX Runtime version: %w", nativeError{err})
	}
	return version, nil
}

// nativeError keeps native diagnostics, which can contain input, out of messages.
type nativeError struct{ cause error }

func (e nativeError) Error() string { return "native operation failed" }
func (e nativeError) Unwrap() error { return e.cause }
