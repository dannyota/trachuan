package retrieval

import (
	"context"
	"math/rand"
	"strings"
	"testing"
)

func TestReadMatcher_MatchesCountAndIndex(t *testing.T) {
	random := rand.New(rand.NewSource(42))
	for trial := 0; trial < 200; trial++ {
		alphabet := "abđ "
		runes := []rune(alphabet)
		word := func(n int) string {
			r := make([]rune, n)
			for i := range r {
				r[i] = runes[random.Intn(len(runes))]
			}
			return string(r)
		}
		text := word(50)
		patterns := []string{"a", "aa", "aaa", "đ", "ab", "b", word(1), word(2), word(3), word(4)}
		m := newReadMatcher()
		ids := make([]int, len(patterns))
		for i, p := range patterns {
			var err error
			ids[i], err = m.add(context.Background(), p)
			if err != nil {
				t.Fatal(err)
			}
		}
		if err := m.scan(context.Background(), text); err != nil {
			t.Fatal(err)
		}
		for i, p := range patterns {
			got := m.matches[ids[i]]
			count := min(strings.Count(text, p), 2)
			if got.count != count || got.first != strings.Index(text, p) {
				t.Fatalf("trial %d pattern %q: got %+v, want count %d at %d", trial, p, got, count, strings.Index(text, p))
			}
		}
	}
}
