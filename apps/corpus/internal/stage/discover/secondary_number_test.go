package discover

import "testing"

func TestSecondary_WireNumberKeepsOfficialSpelling(t *testing.T) {
	if got := secondaryWireNumber(" 142 / 2026 / NĐ - CР "); got != "142/2026/NĐ-CP" {
		t.Fatalf("wire number=%q", got)
	}
}

func TestSecondary_NamedWireDefinitionCleansNumber(t *testing.T) {
	d := secondaryDefinition{Kind: "named", Number: "142 / 2026 / NĐ - CР"}
	if got := secondarySearchText(d); got != "142/2026/NĐ-CP" {
		t.Fatalf("wire search=%q", got)
	}
}
