//go:build ort

package onnx

import (
	"context"
	"fmt"

	ort "github.com/microsoft/onnxruntime/go/onnxruntime"
)

type modelSession interface {
	RunWithOptions(context.Context, *ort.RunOptions, map[string]*ort.Tensor, []string) (map[string]*ort.Tensor, error)
	Close() error
}

func (m *Model) run(ctx context.Context, inputs map[string]*ort.Tensor) (map[string]*ort.Tensor, error) {
	options, err := ort.NewRunOptions()
	if err != nil {
		return nil, fmt.Errorf("create ONNX run options: %w", nativeError{err})
	}
	defer func() { _ = options.Close() }()
	stopped := make(chan struct{})
	joined := make(chan error, 1)
	done := ctx.Done()
	go func() {
		select {
		case <-done:
			joined <- options.SetTerminate()
		case <-stopped:
			joined <- nil
		}
	}()
	// This watcher owns termination. The binding must not create a second watcher
	// or return early on cancellation before native Run has released its inputs.
	outputs, runErr := m.session.RunWithOptions(context.WithoutCancel(ctx), options, inputs, []string{m.outputName})
	close(stopped)
	terminateErr := <-joined
	if err := ctx.Err(); err != nil {
		for _, output := range outputs {
			_ = output.Close()
		}
		return nil, err
	}
	if runErr != nil {
		return nil, fmt.Errorf("run ONNX model: %w", nativeError{runErr})
	}
	if terminateErr != nil {
		for _, output := range outputs {
			_ = output.Close()
		}
		return nil, fmt.Errorf("terminate ONNX run: %w", nativeError{terminateErr})
	}
	return outputs, nil
}
