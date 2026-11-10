//go:build ort

// Package tokenizer serializes access to the native Hugging Face tokenizer.
package tokenizer

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/daulet/tokenizers"
)

// Tokenizer owns a native tokenizer and serializes encoding and cleanup.
type Tokenizer struct {
	mu     sync.Mutex
	native *tokenizers.Tokenizer
}

// Open loads a tokenizer with saved truncation and padding disabled.
func Open(path string) (*Tokenizer, error) {
	if strings.ContainsRune(path, 0) {
		return nil, errors.New("open tokenizer: invalid path")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read tokenizer: %w", nativeError{err})
	}
	var settings struct{ Truncation, Padding json.RawMessage }
	if err = json.Unmarshal(data, &settings); err != nil {
		return nil, fmt.Errorf("read tokenizer settings: %w", nativeError{err})
	}
	for _, setting := range []json.RawMessage{settings.Truncation, settings.Padding} {
		if len(setting) != 0 && string(setting) != "null" {
			return nil, errors.New("open tokenizer: truncation and padding must be disabled")
		}
	}
	native, err := tokenizers.FromFile(path)
	if err != nil {
		return nil, fmt.Errorf("open tokenizer: %w", nativeError{err})
	}
	return &Tokenizer{native: native}, nil
}

// Encode returns every token id, including special tokens, without truncation.
func (t *Tokenizer) Encode(text string) ([]int64, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.native == nil {
		return nil, errors.New("encode tokenizer: tokenizer is closed")
	}
	// The binding passes a NUL-terminated UTF-8 C string. Reject values it
	// cannot represent rather than silently tokenizing only a prefix.
	if strings.ContainsRune(text, 0) || !utf8.ValidString(text) {
		return nil, errors.New("encode tokenizer: input is not representable as a UTF-8 C string")
	}
	ids, _, err := t.native.EncodeErr(text, true)
	if err != nil {
		return nil, fmt.Errorf("encode tokenizer: %w", nativeError{err})
	}
	result := make([]int64, len(ids))
	for i, id := range ids {
		result[i] = int64(id)
	}
	return result, nil
}

// Close releases the native tokenizer after any active encoding finishes.
func (t *Tokenizer) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.native == nil {
		return nil
	}
	err := t.native.Close()
	t.native = nil
	if err != nil {
		return fmt.Errorf("close tokenizer: %w", nativeError{err})
	}
	return nil
}

// nativeError keeps native diagnostics, which can contain input, out of messages.
type nativeError struct{ cause error }

func (e nativeError) Error() string { return "native operation failed" }
func (e nativeError) Unwrap() error { return e.cause }
