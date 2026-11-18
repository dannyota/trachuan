package detector

import (
	"errors"
	"testing"
)

func TestCrossSignal_RegisteredWithDeclaredFields(t *testing.T) {
	want := map[string]struct {
		subject  string
		severity Severity
	}{
		"raw_field_not_normalized":          {"document", Info},
		"partial_without_provision_effect":  {"document", Info},
		"unmatched_operative_claim":         {"claim", Review},
		"relation_kind_disagreement":        {"relation", Review},
		"waived_document_without_relations": {"document", Info},
		"external_report":                   {"report", Review},
	}
	for _, id := range CrossSignalIDs() {
		w, ok := want[id]
		if !ok {
			t.Fatalf("unexpected cross-signal detector %s", id)
		}
		d, found := Lookup(id)
		if !found || d.Subject != w.subject || d.Severity != w.severity || d.Version != 1 || d.Semantics != 1 {
			t.Errorf("%s = %+v, %v", id, d, found)
		}
		delete(want, id)
	}
	if len(want) != 0 {
		t.Fatalf("missing cross-signal detectors %v", want)
	}
}

func TestCrossSignal_NoneBlocking(t *testing.T) {
	for _, id := range CrossSignalIDs() {
		if d, _ := Lookup(id); d.Severity == Blocking {
			t.Errorf("%s is blocking", id)
		}
	}
}

func TestValidateKind_RefusesCrossSignalDetector(t *testing.T) {
	for _, id := range CrossSignalIDs() {
		if err := ValidateKind(id); !errors.Is(err, ErrUnregisteredGap) {
			t.Errorf("ValidateKind(%s) = %v", id, err)
		}
	}
}
