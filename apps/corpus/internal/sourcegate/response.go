package sourcegate

import (
	"bufio"
	"io"
)

// ResponsePrefix returns at most 4096 bytes for inspection and a reader that
// replays the full body, including any terminal read error after the prefix.
func ResponsePrefix(body io.Reader) ([]byte, io.Reader) {
	reader := bufio.NewReaderSize(body, 4096)
	prefix, err := reader.Peek(4096)
	if err == nil {
		return prefix, reader
	}
	// Peek clears its read error, so replay it after the buffered bytes.
	return prefix, io.MultiReader(io.LimitReader(reader, int64(len(prefix))), responseReadError{err})
}

type responseReadError struct{ err error }

func (r responseReadError) Read([]byte) (int, error) { return 0, r.err }
