//go:build !ort

// Package tokenizer provides a native tokenizer when built with tag ort.
package tokenizer

// Tokenizer is unavailable without the ort build tag.
type Tokenizer struct{}

// Open fails because native support is unavailable.
func Open(string) (*Tokenizer, error) { return nil, ErrNativeUnavailable }

// Encode fails because native support is unavailable.
func (*Tokenizer) Encode(string) ([]int64, error) { return nil, ErrNativeUnavailable }

// Close has no native resources to release.
func (*Tokenizer) Close() error { return nil }
