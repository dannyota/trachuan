package onnx

import "errors"

// ErrNativeUnavailable means the binary was built without native support.
var ErrNativeUnavailable = errors.New("native ONNX Runtime unavailable: build with -tags ort")
