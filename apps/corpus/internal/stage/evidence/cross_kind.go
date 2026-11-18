package evidence

import (
	"context"
	"slices"
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/detector"
)

type kindDetector struct{}

func (kindDetector) Spec() detector.Detector { return specOf(detector.RelationKindDisagreement) }

func (kindDetector) Detect(ctx context.Context, in Inputs) (Observation, error) {
	return detectKindDisagreements(ctx, sqlRelease{in.Release})
}

type pairKinds struct{ text, diagram map[string]bool }

type pairKey struct{ source, target string }

func isTextRow(method string) bool { return strings.HasPrefix(method, "text_rule/") }

func isDiagramRow(method string) bool {
	return strings.Contains(method, "vbpl_diagram") || strings.Contains(method, "vbpl_references")
}

func documentedLegitimate(textKind string, diagram map[string]bool) bool {
	switch textKind {
	case "repeals", "replaces", "suspends":
		return diagram["amends_supplements"]
	}
	return false
}

func textRowGapKinds() []string {
	return []string{"relation_text_ambiguous_target", "relation_text_unlinked_target", "relation_text_source_order", "relation_text_identity_conflict"}
}

func detectKindDisagreements(ctx context.Context, rel releaseData) (Observation, error) {
	gaps, err := rel.ExtractGaps(ctx)
	if err != nil {
		return Observation{}, err
	}
	textGaps, err := rel.TextGaps(ctx)
	if err != nil {
		return Observation{}, err
	}
	present := map[string]bool{}
	readable := map[string]bool{}
	obs := Observation{}
	err = rel.Documents(ctx, func(d releaseDocument) error {
		present[d.Key] = true
		blocked, headings := gapState(gaps[d.Key])
		if d.Baseline || neverRead(d) || !d.HasProvisions || blocked || headings || textGaps[d.Key] {
			obs.NotObservable++
			return nil
		}
		obs.Observed++
		readable[d.Key] = true
		return nil
	})
	if err != nil {
		return Observation{}, err
	}
	pairs := map[pairKey]*pairKinds{}
	err = rel.PairRelations(ctx, func(p pairRelation) error {
		text, diagram := isTextRow(p.Method), isDiagramRow(p.Method) && p.Kind != "unknown"
		if !text && !diagram {
			return nil
		}
		key := pairKey{p.Source, p.Target}
		kinds := pairs[key]
		if kinds == nil {
			kinds = &pairKinds{text: map[string]bool{}, diagram: map[string]bool{}}
			pairs[key] = kinds
		}
		if text {
			kinds.text[p.Kind] = true
		}
		if diagram {
			kinds.diagram[p.Kind] = true
		}
		return nil
	})
	if err != nil {
		return Observation{}, err
	}
	keys := make([]pairKey, 0, len(pairs))
	for key := range pairs {
		keys = append(keys, key)
	}
	slices.SortFunc(keys, func(a, b pairKey) int {
		return strings.Compare(a.source+"\x00"+a.target, b.source+"\x00"+b.target)
	})
	for _, key := range keys {
		kinds := pairs[key]
		if len(kinds.diagram) == 0 {
			continue
		}
		diagramKinds := sortedKinds(kinds.diagram)
		for _, textKind := range sortedKinds(kinds.text) {
			switch {
			case kinds.diagram[textKind]:
			case documentedLegitimate(textKind, kinds.diagram):
				obs.Excluded++
			default:
				obs.Findings = append(obs.Findings, Fired{
					Subject:           Subject{Type: detector.SubjectRelation, Fields: []string{key.source, key.target, textKind}},
					FingerprintInputs: []string{textKind, strings.Join(diagramKinds, ",")},
				})
			}
		}
	}
	obs.ObservedCompletely = func(s Subject) bool {
		return len(s.Fields) == 3 && readable[s.Fields[0]] && present[s.Fields[1]]
	}
	return obs, nil
}

func sortedKinds(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for kind := range set {
		out = append(out, kind)
	}
	slices.Sort(out)
	return out
}
