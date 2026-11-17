package evaluate

import (
	"bytes"
	"encoding/json"
)

func compareInvariant(caller string, before, after, fullBefore, fullAfter []invariantObservation) InvariantCaller {
	r := InvariantCaller{Caller: caller, Classes: map[string]InvariantClass{}, Calibration: "unavailable", Failures: []InvariantFailure{}, Pass: true}
	positives := map[string]bool{}
	successfulBefore, successfulAfter := successfulObservations(fullBefore, fullAfter)
	original, mutated := privateObservations(successfulBefore), privateObservations(successfulAfter)
	for _, class := range privateClasses() {
		positives[class] = !bytes.Equal(original[class], mutated[class])
	}
	for _, class := range privateClasses() {
		r.Classes[class] = InvariantClass{Status: "evaluated", PositiveControl: positives[class], Pass: positives[class]}
		if !positives[class] {
			r.Pass = false
			r.Failures = append(r.Failures, InvariantFailure{Operation: "positive_control", Class: class})
		}
	}
	r.Failures = append(r.Failures, comparisonFailures(before, after)...)
	r.Failures = append(r.Failures, observationFailures(fullBefore, fullAfter)...)
	for _, failure := range r.Failures {
		r.Pass = false
		if c, ok := r.Classes[failure.Class]; ok {
			c.Pass = false
			r.Classes[failure.Class] = c
		}
	}
	return withFailureCauses(r)
}
func comparisonFailures(before, after []invariantObservation) []InvariantFailure {
	failures := []InvariantFailure{}
	failures = append(failures, observationFailures(before, after)...)
	before, after = successfulObservations(before, after)
	if len(before) != len(after) {
		failures = append(failures, InvariantFailure{Operation: "response_count", Class: "all"})
	}
	for i, a := range before {
		if i >= len(after) {
			break
		}
		b := after[i]
		if a.Operation != b.Operation || a.CaseID != b.CaseID || a.Mode != b.Mode || a.Page != b.Page || a.Repeat != b.Repeat || !bytes.Equal(a.Value, b.Value) {
			class := operationClass(a.Operation)
			failures = append(failures, InvariantFailure{Operation: a.Operation, Class: class, CaseID: a.CaseID, Mode: a.Mode, Page: a.Page, Repeat: a.Repeat})
		}
	}
	return failures
}

func privateObservations(observations []invariantObservation) map[string][]byte {
	values := map[string][]json.RawMessage{}
	for _, o := range observations {
		class := operationClass(o.Operation)
		var raw []byte
		switch o.Operation {
		case "read":
			raw = o.Text
		case "search":
			raw = o.Scores
		case "mappings":
			raw = o.Evidence
		default:
			raw = o.Value
		}
		if o.Operation == "read" || o.Operation == "search" || o.Operation == "mappings" {
			var parts []json.RawMessage
			if json.Unmarshal(raw, &parts) == nil {
				values[class] = append(values[class], parts...)
			}
		} else {
			values[class] = append(values[class], json.RawMessage(raw))
		}
	}
	result := map[string][]byte{}
	for _, class := range privateClasses() {
		result[class], _ = json.Marshal(values[class])
	}
	return result
}

func observationFailures(sets ...[]invariantObservation) []InvariantFailure {
	failures := []InvariantFailure{}
	for _, observations := range sets {
		for _, o := range observations {
			if o.Error && !o.Diagnostic {
				failures = append(failures, InvariantFailure{Operation: o.Operation, Class: operationClass(o.Operation), CaseID: o.CaseID, Mode: o.Mode, Page: o.Page, Repeat: o.Repeat})
			}
		}
	}
	return failures
}
