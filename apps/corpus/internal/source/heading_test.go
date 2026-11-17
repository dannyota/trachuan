package source

import (
	"strings"
	"testing"
)

func TestHeadingExclusions_ClosedSchema(t *testing.T) {
	valid := `[{key: 'vn:vbpl:1', anchor: dieu-1, reason: operative, reviewed: '2026-11-16'}]`
	for _, data := range []string{"", "[]", valid} {
		if _, err := ParseHeadingExclusions([]byte(data)); err != nil {
			t.Fatalf("%s: %v", data, err)
		}
	}
	for _, data := range []string{"null", "{}", "---\n[]\n---\n[]", strings.Replace(valid, "reason:", "unknown:", 1), strings.Replace(valid, "dieu-1", "''", 1), strings.Replace(valid, "2026-11-16", "2026-02-30", 1), strings.Replace(valid, "operative", "''", 1), strings.Replace(valid, "vn:vbpl:1", "bad", 1), strings.Replace(valid, "reviewed:", "anchor: x, reviewed:", 1), "[" + strings.Trim(valid, "[]") + "," + strings.Trim(valid, "[]") + "]"} {
		if _, err := ParseHeadingExclusions([]byte(data)); err == nil {
			t.Fatalf("accepted %s", data)
		}
	}
}

func TestHeadingReviews_ClosedSchema(t *testing.T) {
	valid := `[{release: '20990101_1', projection: 1, scan_sha256: '` + strings.Repeat("a", 64) + `', exclusions_sha256: '` + strings.Repeat("b", 64) + `', reviewer: coordinator, date: '2026-11-16'}]`
	for _, data := range []string{"", "[]", valid} {
		if _, err := ParseHeadingReviews([]byte(data)); err != nil {
			t.Fatal(err)
		}
	}
	for _, data := range []string{"null", "{}", valid + "\n---\n[]", strings.Replace(valid, "projection: 1", "projection: 2", 1), strings.Replace(valid, "projection: 1", "projection: 1.0", 1), strings.Replace(valid, "projection: 1", "projection: true", 1), strings.Replace(valid, "coordinator", "Person Name", 1), strings.Replace(valid, "date:", "extra:", 1), strings.Replace(valid, "2026-11-16", "2026-02-30", 1), strings.Replace(valid, strings.Repeat("a", 64), "ABC", 1), "[" + strings.Trim(valid, "[]") + "," + strings.Trim(valid, "[]") + "]"} {
		if _, err := ParseHeadingReviews([]byte(data)); err == nil {
			t.Fatalf("accepted %s", data)
		}
	}
}
