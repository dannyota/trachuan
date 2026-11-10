package testtemporal

import (
	"regexp"
	"testing"
)

func TestTemporalRelease_Valid2099(t *testing.T) {
	pattern := regexp.MustCompile(`^2099[0-9]{4}(_[0-9]{1,2})?$`)
	seen := make(map[string]bool)
	for i := 0; i < 366; i++ {
		rel := Release()
		if !pattern.MatchString(rel) || seen[rel] {
			t.Fatalf("invalid or repeated release %q", rel)
		}
		if (i == 0 && rel != "20990101") || (i == 364 && rel != "20991231") || (i == 365 && rel != "20990101_1") {
			t.Fatalf("release %d = %q", i, rel)
		}
		seen[rel] = true
	}
}
