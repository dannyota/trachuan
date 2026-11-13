package embedding

import (
	"fmt"
	"time"
)

type Error struct {
	RetryAfter        *time.Duration
	Status            int
	Code              string
	Index, TokenCount *int
}

func (e *Error) Error() string { return fmt.Sprintf("embedder HTTP %d: %s", e.Status, e.Code) }
