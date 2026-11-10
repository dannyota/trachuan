package tokenizer

import "errors"

// ErrNativeUnavailable means the binary was built without native support.
var ErrNativeUnavailable = errors.New("native tokenizer unavailable: build with -tags ort")
