package stage

import (
	"context"
	"errors"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/detector"
)

func TestCommitItem_RefusesUnregisteredGapKind(t *testing.T) {
	for _, kind := range []string{"future_gap", ""} {
		_, err := CommitItem(context.Background(), nil, Commit{Stage: "fetch", Gaps: []Gap{{Kind: kind, Detail: "x"}}})
		if !errors.Is(err, detector.ErrUnregisteredGap) || !strings.Contains(err.Error(), `"`+kind+`"`) {
			t.Errorf("kind %q: err = %v", kind, err)
		}
	}
}
