package sourcegate

import (
	"bytes"
	"context"
	"errors"
	"io"
	"time"
)

// ResponsePrefix inspects at most 4096 bytes and replays the full body and error.
// A success response uses only its request deadline. Other statuses get at most
// five seconds of inspection; expiry closes their body. A recognized challenge
// ends inspection as soon as its bytes arrive.
func ResponsePrefix(ctx context.Context, body io.Reader, closer io.Closer, status, success int, challenge func([]byte) bool) ([]byte, io.Reader) {
	prefix, replay, _ := ResponsePrefixComplete(ctx, body, closer, status, success, challenge)
	return prefix, replay
}

// ResponsePrefixComplete also reports whether the body ended or the prefix limit was reached.
func ResponsePrefixComplete(ctx context.Context, body io.Reader, closer io.Closer, status, success int, challenge func([]byte) bool) ([]byte, io.Reader, bool) {
	if status == success {
		prefix, err, complete := readResponsePrefix(body, challenge)
		head, replay := replayResponsePrefix(prefix, body, err)
		return head, replay, complete
	}
	inspection, cancel := context.WithTimeout(ctx, 5*time.Second)
	done := make(chan struct{})
	stop := context.AfterFunc(inspection, func() { _ = closer.Close(); close(done) })
	prefix, err, complete := readResponsePrefix(body, challenge)
	if !stop() {
		<-done
	}
	if inspection.Err() != nil {
		err = inspection.Err()
	}
	cancel()
	head, replay := replayResponsePrefix(prefix, body, err)
	return head, replay, complete
}

func readResponsePrefix(body io.Reader, challenge func([]byte) bool) ([]byte, error, bool) {
	prefix := make([]byte, 4096)
	size := 0
	for size < len(prefix) {
		n, err := body.Read(prefix[size:])
		size += n
		if err != nil {
			return prefix[:size], err, errors.Is(err, io.EOF) || size == len(prefix)
		}
		if challenge != nil && challenge(prefix[:size]) {
			return prefix[:size], nil, size == len(prefix)
		}
		if n == 0 {
			return prefix[:size], io.ErrNoProgress, false
		}
	}
	return prefix, nil, true
}
func replayResponsePrefix(prefix []byte, body io.Reader, err error) ([]byte, io.Reader) {
	if err != nil {
		body = responseReadError{err}
	}
	return prefix, io.MultiReader(bytes.NewReader(prefix), body)
}

type responseReadError struct{ err error }

func (r responseReadError) Read([]byte) (int, error) { return 0, r.err }
