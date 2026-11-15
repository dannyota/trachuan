package build

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTaxonomyStats_ReportsInheritance(t *testing.T) {
	data, err := json.Marshal(TaxonomyStats{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"inherited"`) {
		t.Fatalf("missing inherited count in %s", data)
	}
}

func TestInheritanceRows_ReportsPrecision(t *testing.T) {
	gate := inheritanceGate{Rows: [3]inheritanceRow{{Numerator: 19, Denominator: 20}}}
	data, err := json.Marshal(gate)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"value":0.95`) || !strings.Contains(string(data), `"value":null`) {
		t.Fatalf("precision values missing: %s", data)
	}
}

func TestInheritanceGate_ExplainsUnavailable(t *testing.T) {
	gate := inheritArticles(nil, nil, nil).gate
	data, err := json.Marshal(gate)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"reason":"fewer_than_100_articles"`) {
		t.Fatalf("missing reason: %s", data)
	}
}

func TestInheritanceGate_RejectsInconsistentStatus(t *testing.T) {
	gate := inheritArticles(nil, nil, nil).gate
	gate.Status = "pass"
	if validInheritanceGate(gate) {
		t.Fatal("accepted unavailable slice as passing")
	}
}
