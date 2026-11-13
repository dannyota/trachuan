//go:build integration

package build

import (
	"context"
	"strings"
	"testing"
)

func TestDatabaseMetrics_SparseOutOfLine(t *testing.T) {
	f := fixture(t)
	ctx := context.Background()
	hash := strings.Repeat("a", 64)
	if _, err := f.corpus.Exec(ctx, `INSERT INTO release VALUES (true,'vn','r',5,'','{}','{}','{}','{}','{}',$1,$1,$1,'{}',now())`, hash); err != nil {
		t.Fatal(err)
	}
	counts, _, _, err := databaseMetrics(ctx, f.corpus, Counts{})
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := counts["sparse_out_of_line"]; !ok || got != 0 {
		t.Fatalf("sparse_out_of_line = %d, present = %t, want 0", got, ok)
	}
}
