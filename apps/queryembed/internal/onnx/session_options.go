//go:build ort

package onnx

import (
	"errors"
	"fmt"

	ort "github.com/microsoft/onnxruntime/go/onnxruntime"
)

type sessionOptions interface {
	SetIntraOpNumThreads(int) error
	SetInterOpNumThreads(int) error
	SetExecutionMode(ort.ExecutionMode) error
	SetGraphOptimizationLevel(ort.GraphOptimizationLevel) error
	DisableCpuMemArena() error
}

func configureSession(options sessionOptions, o Options) error {
	if o.IntraOp <= 0 || o.InterOp <= 0 {
		return errors.New("configure ONNX session: thread counts must be positive")
	}
	for _, set := range []func() error{
		func() error { return options.SetIntraOpNumThreads(o.IntraOp) },
		func() error { return options.SetInterOpNumThreads(o.InterOp) },
		func() error { return options.SetExecutionMode(ort.ExecutionModeSequential) },
		func() error { return options.SetGraphOptimizationLevel(ort.GraphOptimizationLevelAll) },
		options.DisableCpuMemArena,
	} {
		if err := set(); err != nil {
			return fmt.Errorf("configure ONNX session: %w", nativeError{err})
		}
	}
	// No other providers are appended, so ONNX Runtime uses its CPU provider.
	return nil
}

func newSessionOptions(o Options) (*ort.SessionOptions, error) {
	options, err := ort.NewSessionOptions()
	if err != nil {
		return nil, fmt.Errorf("create ONNX session options: %w", nativeError{err})
	}
	if err := configureSession(options, o); err != nil {
		_ = options.Close()
		return nil, err
	}
	return options, nil
}
