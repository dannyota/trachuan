package embedding

import "fmt"

// Error describes a rejected embedder request.
type Error struct {
	Status            int
	Code              string
	Index, TokenCount *int
}

// Error returns the status and fixed service error code.
func (e *Error) Error() string { return fmt.Sprintf("embedder HTTP %d: %s", e.Status, e.Code) }
