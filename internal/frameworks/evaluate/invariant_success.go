package evaluate

type observationOperation struct{ operation, caseID, mode string }

func successfulObservations(before, after []invariantObservation) ([]invariantObservation, []invariantObservation) {
	failed := map[observationOperation]bool{}
	for _, observations := range [][]invariantObservation{before, after} {
		for _, o := range observations {
			if o.Error {
				failed[observationOperation{o.Operation, o.CaseID, o.Mode}] = true
			}
		}
	}
	keep := func(observations []invariantObservation) []invariantObservation {
		result := make([]invariantObservation, 0, len(observations))
		for _, o := range observations {
			if !failed[observationOperation{o.Operation, o.CaseID, o.Mode}] {
				result = append(result, o)
			}
		}
		return result
	}
	return keep(before), keep(after)
}
