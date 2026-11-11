package build

import (
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/stage/fetch"
)

func TestExtractLineage_NonTreeRequiresTextRecord(t *testing.T) {
	dir := t.TempDir()
	if err := fetch.CheckExtractLineage(dir, "fetch", fetch.ExtractInputHash("fetch", "none")); err == nil {
		t.Fatal("non-tree input without text record accepted")
	}
}
