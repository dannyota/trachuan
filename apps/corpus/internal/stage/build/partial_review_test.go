package build

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestPartialDocuments_ActorsAndOrder(t *testing.T) {
	docs := map[string]validityDocument{
		"a": {key: "a", number: "A", class: "relation_context", sourceClass: "partial", statusCode: "HHL1P"},
		"b": {key: "b", number: "B", class: "primary", sourceClass: "partial", statusCode: "HHL2P"},
		"c": {key: "c", number: "C", sourceClass: "in_force"},
		"d": {key: "d", number: "D", sourceClass: "in_force"},
	}
	relations := []validityRelation{{source: "d", target: "a", kind: "repeals"}, {source: "c", target: "a", kind: "suspends"}, {source: "c", target: "a", kind: "amends_supplements"}, {source: "d", target: "a", kind: "replaces"}, {source: "c", target: "b", kind: "references"}}
	want := []partialDocument{
		{Key: "a", Number: "A", Class: "relation_context", StatusCode: "HHL1P", Acting: []partialActor{{"c", "C", "amends_supplements"}, {"c", "C", "suspends"}, {"d", "D", "repeals"}, {"d", "D", "replaces"}}},
		{Key: "b", Number: "B", Class: "primary", StatusCode: "HHL2P", Acting: []partialActor{}},
	}
	got := partialDocuments(docs, relations)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
	data, err := encodeValidityReview(nil, got)
	if err != nil {
		t.Fatal(err)
	}
	var report struct {
		Partial []partialDocument `json:"partial_documents"`
	}
	if err := json.Unmarshal(data, &report); err != nil || !reflect.DeepEqual(report.Partial, want) {
		t.Fatalf("review %s: %v", data, err)
	}
}
