package testtemporal

import (
	"regexp"
	"testing"
	"time"
)

func TestTemporalRelease_Valid2099(t *testing.T) {
	pattern := regexp.MustCompile(`^2099[0-9]{4}(_[0-9]{1,2})?$`)
	seen := make(map[string]bool)
	for i := 0; i < 366; i++ {
		rel := Release()
		if !pattern.MatchString(rel) || seen[rel] {
			t.Fatalf("invalid or repeated release %q", rel)
		}
		if _, err := time.Parse("20060102", rel[:8]); err != nil {
			t.Fatalf("invalid release date %q: %v", rel, err)
		}
		seen[rel] = true
	}
}
