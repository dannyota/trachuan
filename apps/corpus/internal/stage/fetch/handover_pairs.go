package fetch

import "danny.vn/trachuan/apps/corpus/internal/stage/discover"

func rawHandOverValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
func recordIdentity(r discover.HandOverRecord) handOverIdentity {
	return normalizedIdentity(r.Number, r.Issuer, r.Date)
}
func comparisonIdentity(c discover.HandOverComparison) handOverIdentity {
	return normalizedIdentity(rawHandOverValue(c.Number), rawHandOverValue(c.Issuer), rawHandOverValue(c.Date))
}
func commonIdentity(a, b handOverIdentity) handOverIdentity {
	if a.number != b.number {
		a.number = ""
	}
	if a.issuer != b.issuer {
		a.issuer = ""
	}
	if a.date != b.date {
		a.date = ""
	}
	return a
}
func (e *handOverEvidence) addRecord(r handOverRecord, _ map[string]handOverFetched) {
	identity := recordIdentity(r.record)
	if !r.complete {
		identity = handOverIdentity{}
	}
	for _, prior := range e.secondaryHistory {
		if identityConflict(prior, identity) {
			e.conflict = true
		}
	}
	e.secondaryHistory = append(e.secondaryHistory, identity)
	if e.hasSecondary {
		e.secondary = commonIdentity(e.secondary, identity)
	} else {
		e.secondary = identity
		e.hasSecondary = true
	}
	e.refs = append(e.refs, r.refs...)
	for _, comparison := range r.record.Comparisons {
		key := "vn:vbpl:" + comparison.ID
		if comparison.ID == "" {
			continue
		}
		id := comparisonIdentity(comparison)
		if comparison.State != "match" && comparison.State != "ambiguous" && identity.number != "" && id.number != "" && identity.number != id.number {
			continue
		}
		e.pairs[key] = append(e.pairs[key], handOverObservation{identity: id, state: comparison.State, complete: r.complete && r.record.ProbeState == "complete"})
	}
}
func (e *handOverEvidence) pairOutcomes(primary map[string]handOverFetched) []handOverPair {
	for key, fetched := range primary {
		if fetched.done && e.secondary.number != "" && fetched.identity.number == e.secondary.number {
			if _, ok := e.pairs[key]; !ok {
				e.pairs[key] = nil
			}
		}
	}
	pairs := make([]handOverPair, 0, len(e.pairs))
	for key, observations := range e.pairs {
		fetched := primary[key]
		conflict := e.conflict
		for i, observation := range observations {
			for _, prior := range observations[:i] {
				if identityConflict(prior.identity, observation.identity) {
					conflict = true
				}
			}
			if fetched.done && identityConflict(fetched.identity, observation.identity) {
				conflict = true
			}
		}
		state := "ambiguous"
		if !conflict && fetched.done {
			state = compareHandOver(e.secondary, fetched.identity)
		} else if !conflict && len(observations) > 0 {
			state = compareHandOver(e.secondary, observations[0].identity)
			for _, observation := range observations {
				if !observation.complete || observation.state != "match" && state == "match" {
					state = "ambiguous"
				}
				if compareHandOver(e.secondary, observation.identity) != state {
					state = "ambiguous"
				}
			}
		}
		for _, observation := range observations {
			if observation.state == "ambiguous" {
				state = "ambiguous"
			}
		}
		if conflict {
			state = "ambiguous"
		}
		if state != "different_number" {
			e.refs = append(e.refs, fetched.refs...)
		}
		pairs = append(pairs, handOverPair{key: key, state: state})
	}
	return pairs
}
