package embedding

import "fmt"

type Error struct {
	RetryAfter        *int
	Status            int
	Code              string
	Index, TokenCount *int
}

func (e *Error) Error() string { return fmt.Sprintf("embedder HTTP %d: %s", e.Status, e.Code) }
