package build

import (
	"danny.vn/trachuan/internal/retrieval/lexical"
	"testing"
)

func TestSparseText_FormatsDimensions(t *testing.T) {
	got := sparseText(lexical.Vector{{Index: 1, Value: 0.5}, {Index: 4, Value: 2}})
	if got != "{1:0.5,4:2}/1048576" {
		t.Fatalf("got %q", got)
	}
}
