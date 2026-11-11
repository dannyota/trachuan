package retrieval

import (
	"context"
	"testing"
)

func TestGaps_RejectsInvalidPagingBeforeDatabase(t *testing.T) {
	e := &Engine{}
	for _, tt := range []struct {
		after int64
		limit int
	}{{-1, 1}, {0, -1}, {0, 201}} {
		if _, _, err := e.Gaps(context.Background(), "", tt.after, tt.limit); err == nil {
			t.Fatalf("accepted %+v", tt)
		}
	}
}
