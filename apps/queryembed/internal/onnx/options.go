// Package onnx loads an ONNX Runtime CPU session when built with tag ort.
package onnx

// Options configures the native library and CPU session threads.
type Options struct {
	LibraryPath      string
	IntraOp, InterOp int
}

// QueryOptions returns the fixed query thread budget.
func QueryOptions(lib string) Options { return Options{LibraryPath: lib, IntraOp: 4, InterOp: 1} }
