package sourcegate

import (
	"bufio"
	"context"
	"io"
	"time"
)

// ResponsePrefix returns at most 4096 bytes for inspection and a reader that
// replays the full body, including any terminal read error after the prefix.
// Inspection takes at most five seconds or the remaining request deadline.
// Expiry closes the response body to unblock its read.
func ResponsePrefix(ctx context.Context, body io.Reader, closer io.Closer) ([]byte, io.Reader) {
	inspection, cancel := context.WithTimeout(ctx, 5*time.Second)
	done := make(chan struct{})
	stop := context.AfterFunc(inspection, func() { _ = closer.Close(); close(done) })
	reader := bufio.NewReaderSize(body, 4096)
	prefix, err := reader.Peek(4096)
	if !stop() {
		<-done
	}
	if inspection.Err() != nil {
		err = inspection.Err()
	}
	cancel()
	if err == nil {
		return prefix, reader
	}
	// Peek clears its read error, so replay it after the buffered bytes.
	return prefix, io.MultiReader(io.LimitReader(reader, int64(len(prefix))), responseReadError{err})
}

type responseReadError struct{ err error }

func (r responseReadError) Read([]byte) (int, error) { return 0, r.err }
