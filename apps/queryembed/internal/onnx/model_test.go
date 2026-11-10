//go:build ort

package onnx

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	ort "github.com/microsoft/onnxruntime/go/onnxruntime"
)

func testOptions(t *testing.T) Options {
	t.Helper()
	lib := os.Getenv("QUERYEMBED_ORT_LIB")
	if lib == "" {
		t.Fatal("QUERYEMBED_ORT_LIB must name the native ONNX Runtime library under -tags ort")
	}
	return QueryOptions(lib)
}

func openTestModel(t *testing.T, name string) *Model {
	t.Helper()
	m, err := Open(filepath.Join("../../testdata", name), testOptions(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := m.Close(); err != nil {
			t.Error(err)
		}
	})
	return m
}

func TestModel_RefusesWrongInputsOrOutput(t *testing.T) {
	for _, file := range []string{"wrong_inputs.onnx", "wrong_output.onnx"} {
		t.Run(file, func(t *testing.T) {
			m, err := Open(filepath.Join("../../testdata", file), testOptions(t))
			if err == nil {
				_ = m.Close()
				t.Fatal("Open accepted wrong signature")
			}
		})
	}
}

func TestModel_CLSNormalized(t *testing.T) {
	m := openTestModel(t, "tiny.onnx")
	if m.Dimensions() != 8 {
		t.Fatalf("dimensions = %d", m.Dimensions())
	}
	for _, ids := range [][]int64{{1, 2, 7}, {2, 1}, {1}} {
		vector, err := m.Embed(context.Background(), ids)
		if err != nil {
			t.Fatal(err)
		}
		// Lookup row 1 is [1..8], row 2 is [8..1].
		for i, got := range vector {
			value := i + 1
			if ids[0] == 2 {
				value = 8 - i
			}
			want := float32(value) / float32(math.Sqrt(204))
			if math.Abs(float64(got-want)) > 1e-6 {
				t.Fatalf("component %d = %v, want %v", i, got, want)
			}
		}
		if len(vector) != 8 {
			t.Fatalf("vector length = %d", len(vector))
		}
	}
	if _, err := m.Embed(context.Background(), nil); err == nil {
		t.Fatal("empty ids accepted")
	}
	if _, err := m.Embed(context.Background(), []int64{100}); err == nil {
		t.Fatal("invalid id accepted")
	}
}

func TestModel_RejectsNonFiniteOutput(t *testing.T) {
	m := openTestModel(t, "tiny.onnx")
	for _, ids := range [][]int64{{3}, {4}, {5}, {6}, {1, 3}} {
		if _, err := m.Embed(context.Background(), ids); err == nil {
			t.Fatalf("invalid output accepted for row %d", ids[0])
		}
	}
}

// watchedContext lets the test cancel once the native run watcher is installed.
type watchedContext struct {
	context.Context
	once     sync.Once
	watching chan struct{}
}

func (c *watchedContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.watching) })
	return c.Context.Done()
}

func TestModel_CancelledContextTerminatesRun(t *testing.T) {
	m, err := Open("../../testdata/cancel.onnx", testOptions(t))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	watched := &watchedContext{Context: ctx, watching: make(chan struct{})}
	result := make(chan error, 1)
	go func() { _, runErr := m.Embed(watched, []int64{1, 2}); result <- runErr }()
	<-watched.watching
	cancel()
	select {
	case err := <-result:
		// Register cleanup only after Run returns, so a termination regression
		// fails the test instead of blocking forever in Close.
		t.Cleanup(func() { _ = m.Close() })
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Embed error = %v", err)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("native run did not terminate")
	}
	// Each call owns its terminate flag; cancellation must not poison a later run.
	reuse, stop := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer stop()
	if _, err := m.Embed(reuse, []int64{0}); err != nil {
		t.Fatal(err)
	}
}

func TestModel_ThreadAndArenaOptions(t *testing.T) {
	o := testOptions(t)
	if o.IntraOp != 4 || o.InterOp != 1 {
		t.Fatalf("QueryOptions = %+v", o)
	}
	m := openTestModel(t, "tiny.onnx")
	_ = m
	opts, err := newSessionOptions(o)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = opts.Close() }()
	mode, err := opts.GetExecutionMode()
	if err != nil || mode != ort.ExecutionModeSequential {
		t.Fatalf("execution mode = %v, %v", mode, err)
	}
	// The binding has no getters for thread counts, graph optimization or CPU arena.
	record := &optionRecord{}
	if err := configureSession(record, o); err != nil {
		t.Fatal(err)
	}
	if record.intra != 4 || record.inter != 1 || record.mode != ort.ExecutionModeSequential || record.graph != ort.GraphOptimizationLevelAll || !record.arenaDisabled {
		t.Fatalf("options = %+v", record)
	}
}

type optionRecord struct {
	intra, inter  int
	mode          ort.ExecutionMode
	graph         ort.GraphOptimizationLevel
	arenaDisabled bool
}

func (r *optionRecord) SetIntraOpNumThreads(n int) error           { r.intra = n; return nil }
func (r *optionRecord) SetInterOpNumThreads(n int) error           { r.inter = n; return nil }
func (r *optionRecord) SetExecutionMode(m ort.ExecutionMode) error { r.mode = m; return nil }
func (r *optionRecord) SetGraphOptimizationLevel(g ort.GraphOptimizationLevel) error {
	r.graph = g
	return nil
}
func (r *optionRecord) DisableCpuMemArena() error { r.arenaDisabled = true; return nil }

func TestModel_LibraryPathAndLifetime(t *testing.T) {
	m := openTestModel(t, "tiny.onnx")
	options := testOptions(t)
	options.LibraryPath += ".different"
	if other, err := Open("../../testdata/tiny.onnx", options); err == nil {
		_ = other.Close()
		t.Fatal("different library path accepted")
	}
	version, err := Version()
	if err != nil || version != "1.31.1" {
		t.Fatalf("Version = %q, %v", version, err)
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Embed(context.Background(), []int64{1}); err == nil {
		t.Fatal("closed model accepted")
	}
}

func TestModel_RejectsInvalidSignatureMetadata(t *testing.T) {
	validInputs := []ort.IOInfo{
		{Name: "input_ids", DataType: ort.TensorElementDataTypeInt64, Shape: []int64{-1, -1}},
		{Name: "attention_mask", DataType: ort.TensorElementDataTypeInt64, Shape: []int64{-1, -1}},
	}
	validOutputs := []ort.IOInfo{{Name: "hidden", DataType: ort.TensorElementDataTypeFloat32, Shape: []int64{-1, -1, 8}}}
	for _, tc := range []struct {
		name            string
		inputs, outputs []ort.IOInfo
	}{
		{"extra input", append(append([]ort.IOInfo{}, validInputs...), validInputs[0]), validOutputs},
		{"missing input", validInputs[:1], validOutputs},
		{"duplicate name", []ort.IOInfo{validInputs[0], validInputs[0]}, validOutputs},
		{"input dtype", []ort.IOInfo{{Name: "input_ids", DataType: ort.TensorElementDataTypeFloat32, Shape: []int64{-1, -1}}, validInputs[1]}, validOutputs},
		{"input rank", []ort.IOInfo{{Name: "input_ids", DataType: ort.TensorElementDataTypeInt64, Shape: []int64{-1}}, validInputs[1]}, validOutputs},
		{"no output", validInputs, nil},
		{"output rank", validInputs, []ort.IOInfo{{Name: "hidden", DataType: ort.TensorElementDataTypeFloat32, Shape: []int64{-1, 8}}}},
		{"dynamic dimensions", validInputs, []ort.IOInfo{{Name: "hidden", DataType: ort.TensorElementDataTypeFloat32, Shape: []int64{-1, -1, -1}}}},
		{"zero dimensions", validInputs, []ort.IOInfo{{Name: "hidden", DataType: ort.TensorElementDataTypeFloat32, Shape: []int64{-1, -1, 0}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := validateSignature(tc.inputs, tc.outputs); err == nil {
				t.Fatal("invalid metadata accepted")
			}
		})
	}
}
