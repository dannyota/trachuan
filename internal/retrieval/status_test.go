package retrieval

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestSearch_ReadOnlyGuardBeforeEmbedding(t *testing.T) {
	threshold := 0.1
	e := &Engine{embedder: readOnlyRuntime{}, release: Release{RetrievalRecipe: Recipe1()}}
	e.release.RetrievalRecipe.Abstention.Threshold = &threshold
	for _, query := range []string{"ngân hàng", "ngan hang", ""} {
		_, err := e.Search(context.Background(), Request{Query: query})
		if !errors.Is(err, ErrSearchUnavailable) {
			t.Fatalf("query %q: %v", query, err)
		}
	}
}
func TestOpenForRead_RejectsMissingDatabase(t *testing.T) {
	if _, err := OpenForRead(context.Background(), nil, Options{}); err == nil {
		t.Fatal("accepted missing database")
	}
}
func TestSourceDate_FormatsAndInvalidDates(t *testing.T) {
	for _, tt := range []struct{ raw, want string }{
		{"2024-02-29", "2024-02-29"}, {" 02/01/2025 ", "2025-01-02"},
		{"2024-03-04T00:00:00", "2024-03-04"}, {"2024-01-01T23:30:00Z", "2024-01-02"},
		{"2023-02-29", ""}, {"", ""}, {"0000-01-01", ""}, {"bad", ""},
	} {
		t.Run(tt.raw, func(t *testing.T) {
			got := sourceDate(tt.raw)
			if tt.want == "" {
				if got != nil {
					t.Fatal(got)
				}
				return
			}
			if got == nil || got.Format(time.DateOnly) != tt.want {
				t.Fatalf("%v want %s", got, tt.want)
			}
		})
	}
}
