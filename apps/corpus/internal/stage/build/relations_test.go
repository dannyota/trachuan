package build

import "testing"

func TestResolveNumber_RequiresUniqueMatch(t *testing.T) {
	numbers := map[string][]string{"01/2024/TT": {"vn:vbpl:1", "vn:vbpl:2"}, "02/2024/TT": {"vn:vbpl:3"}}
	if got := resolveNumber(numbers, "01/2024/TT"); got != "" {
		t.Fatalf("ambiguous number resolved to %q", got)
	}
	if got := resolveNumber(numbers, "02/2024/TT"); got != "vn:vbpl:3" {
		t.Fatalf("unique number resolved to %q", got)
	}
}

func TestRelations_NumberIndexUsesLoadedDocuments(t *testing.T) {
	l := loader{docs: map[string]*loadedDocument{"vn:vbpl:1": {number: "01/2024/TT"}, "vn:vbpl:2": {number: "02/2024/TT"}}}
	numbers := l.documentNumbers()
	if got := resolveNumber(numbers, "01/2024/TT"); got != "vn:vbpl:1" {
		t.Fatalf("number resolved to %q", got)
	}
}
