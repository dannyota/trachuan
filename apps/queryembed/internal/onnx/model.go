//go:build ort

package onnx

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"

	ort "github.com/microsoft/onnxruntime/go/onnxruntime"
)

// Model owns a CPU session and permits concurrent inference calls.
type Model struct {
	mu         sync.RWMutex
	session    *ort.Session
	dimensions int
	outputName string
}

// Open loads a CPU model and validates its input and first-output signatures.
func Open(modelPath string, o Options) (*Model, error) {
	if err := initialize(o.LibraryPath); err != nil {
		return nil, err
	}
	options, err := newSessionOptions(o)
	if err != nil {
		return nil, err
	}
	defer func() { _ = options.Close() }()
	session, err := ort.NewSession(modelPath, options)
	if err != nil {
		return nil, fmt.Errorf("open ONNX model: %w", nativeError{err})
	}
	outputs := session.Outputs()
	dimensions, err := validateSignature(session.Inputs(), outputs)
	if err != nil {
		_ = session.Close()
		return nil, err
	}
	return &Model{session: session, dimensions: dimensions, outputName: outputs[0].Name}, nil
}

func validateSignature(inputs, outputs []ort.IOInfo) (int, error) {
	if len(inputs) != 2 {
		return 0, errors.New("ONNX model must have exactly two inputs")
	}
	seen := make(map[string]bool, 2)
	for _, input := range inputs {
		if (input.Name != "input_ids" && input.Name != "attention_mask") || seen[input.Name] || input.DataType != ort.TensorElementDataTypeInt64 || len(input.Shape) != 2 {
			return 0, errors.New("ONNX model inputs must be int64 input_ids and attention_mask of rank two")
		}
		seen[input.Name] = true
	}
	if len(outputs) == 0 || outputs[0].DataType != ort.TensorElementDataTypeFloat32 || len(outputs[0].Shape) != 3 || outputs[0].Shape[2] <= 0 || outputs[0].Shape[2] > math.MaxInt {
		return 0, errors.New("ONNX first output must be float32 batch by sequence by dimensions")
	}
	return int(outputs[0].Shape[2]), nil
}

// Dimensions returns the model's embedding width.
func (m *Model) Dimensions() int { return m.dimensions }

// Embed runs one unpadded input and returns its CLS vector, L2-normalized in float32.
func (m *Model) Embed(ctx context.Context, ids []int64) ([]float32, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, errors.New("embed ONNX model: token ids are empty")
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.session == nil {
		return nil, errors.New("embed ONNX model: model is closed")
	}
	shape := []int64{1, int64(len(ids))}
	input, err := ort.CreateTensor(shape, ids)
	if err != nil {
		return nil, fmt.Errorf("create ONNX input: %w", nativeError{err})
	}
	defer func() { _ = input.Close() }()
	mask := make([]int64, len(ids))
	for i := range mask {
		mask[i] = 1
	}
	attention, err := ort.CreateTensor(shape, mask)
	if err != nil {
		return nil, fmt.Errorf("create ONNX attention mask: %w", nativeError{err})
	}
	defer func() { _ = attention.Close() }()
	outputs, err := m.run(ctx, map[string]*ort.Tensor{"input_ids": input, "attention_mask": attention})
	if err != nil {
		return nil, err
	}
	defer func() {
		for _, output := range outputs {
			_ = output.Close()
		}
	}()
	output := outputs[m.outputName]
	if output == nil {
		return nil, errors.New("embed ONNX model: output is missing")
	}
	actual := output.Shape()
	if len(actual) != 3 || actual[0] != 1 || actual[1] != int64(len(ids)) || actual[2] != int64(m.dimensions) {
		return nil, errors.New("embed ONNX model: output shape differs from input")
	}
	data, err := ort.TensorData[float32](output)
	if err != nil {
		return nil, fmt.Errorf("read ONNX output: %w", nativeError{err})
	}
	for _, value := range data {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return nil, errors.New("embed ONNX model: output is not finite")
		}
	}
	vector := append([]float32(nil), data[:m.dimensions]...)
	var sum float32
	for _, value := range vector {
		sum += value * value
	}
	if sum == 0 || math.IsInf(float64(sum), 0) {
		return nil, errors.New("embed ONNX model: invalid vector norm")
	}
	norm := float32(math.Sqrt(float64(sum)))
	for i := range vector {
		vector[i] /= norm
	}
	return vector, nil
}

// Close waits for active inference calls before releasing the session.
func (m *Model) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.session == nil {
		return nil
	}
	err := m.session.Close()
	m.session = nil
	if err != nil {
		return fmt.Errorf("close ONNX model: %w", nativeError{err})
	}
	return nil
}
