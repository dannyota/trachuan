//go:build !ort

package onnx

import (
	"context"
)

// Model is unavailable without the ort build tag.
type Model struct{}

// OpenBytes fails because native support is unavailable.
func OpenBytes([]byte, Options) (*Model, error) { return nil, ErrNativeUnavailable }

// Embed fails because native support is unavailable.
func (*Model) Embed(context.Context, []int64) ([]float32, error) { return nil, ErrNativeUnavailable }

// Dimensions returns zero because no model is loaded.
func (*Model) Dimensions() int { return 0 }

// Close has no native resources to release.
func (*Model) Close() error { return nil }

// Version fails because native support is unavailable.
func Version() (string, error) { return "", ErrNativeUnavailable }
