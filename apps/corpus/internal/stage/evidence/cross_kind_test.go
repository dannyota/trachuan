package evidence

import (
	"context"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/detector"
)

func kindRelease(pairs ...pairRelation) *fakeRelease {
	return &fakeRelease{
		documents: []releaseDocument{
			{ID: 1, Key: "vn:src", HasProvisions: true},
			{ID: 2, Key: "vn:target", HasProvisions: true},
		},
		pairs: pairs,
	}
}

func diagram(kind string) pairRelation {
	return pairRelation{Source: "vn:src", Target: "vn:target", Kind: kind, Method: "vbpl_diagram"}
}

func text(kind string) pairRelation {
	return pairRelation{Source: "vn:src", Target: "vn:target", Kind: kind, Method: "text_rule/1"}
}

func detectKinds(t *testing.T, rel *fakeRelease) Observation {
	t.Helper()
	obs, err := detectKindDisagreements(context.Background(), rel)
	if err != nil {
		t.Fatal(err)
	}
	return obs
}

func TestRelationKindDisagreement_FiresOnDifferentKinds(t *testing.T) {
	for name, tc := range map[string]struct{ diagram, text string }{
		"repeal against amendment":     {"repeals", "amends_supplements"},
		"replace against amendment":    {"replaces", "amends_supplements"},
		"citation against repeal":      {"cites", "repeals"},
		"legal basis against replace":  {"legal_basis", "replaces"},
		"suspension against amendment": {"suspends", "amends_supplements"},
		"citation against amendment":   {"cites", "amends_supplements"},
		"citation against suspension":  {"cites", "suspends"},
	} {
		t.Run(name, func(t *testing.T) {
			obs := detectKinds(t, kindRelease(diagram(tc.diagram), text(tc.text)))
			if len(obs.Findings) != 1 || obs.Excluded != 0 {
				t.Fatalf("%+v", obs)
			}
			f := obs.Findings[0]
			if f.Subject.Type != "relation" || strings.Join(f.Subject.Fields, "|") != "vn:src|vn:target|"+tc.text || f.Discriminator != "" || f.Severity != "" {
				t.Fatalf("finding %+v", f)
			}
			if err := f.Subject.validate(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRelationKindDisagreement_SilentWhenAKindMatches(t *testing.T) {
	for name, rows := range map[string][]pairRelation{
		"same kind":              {diagram("repeals"), text("repeals")},
		"one of several kinds":   {diagram("cites"), diagram("amends_supplements"), text("amends_supplements")},
		"merged diagram methods": {{Source: "vn:src", Target: "vn:target", Kind: "repeals", Method: "vbpl_references,vbpl_diagram"}, text("repeals")},
		"references only":        {{Source: "vn:src", Target: "vn:target", Kind: "repeals", Method: "vbpl_references"}, text("repeals")},
		"text without a diagram": {text("repeals")},
		"diagram without text":   {diagram("repeals")},
	} {
		t.Run(name, func(t *testing.T) {
			if obs := detectKinds(t, kindRelease(rows...)); len(obs.Findings) != 0 || obs.Excluded != 0 {
				t.Fatalf("%+v", obs)
			}
		})
	}
}

func TestRelationKindDisagreement_EveryDiagramMethodNamesTheDiagramKind(t *testing.T) {
	for _, method := range []string{"vbpl_diagram", "vbpl_references", "vbpl_references,vbpl_diagram"} {
		row := pairRelation{Source: "vn:src", Target: "vn:target", Kind: "cites", Method: method}
		if obs := detectKinds(t, kindRelease(row, text("repeals"))); len(obs.Findings) != 1 {
			t.Errorf("%s: %+v", method, obs.Findings)
		}
	}
	other := pairRelation{Source: "vn:src", Target: "vn:target", Kind: "cites", Method: "chinhphu_rows"}
	if obs := detectKinds(t, kindRelease(other, text("repeals"))); len(obs.Findings) != 0 {
		t.Errorf("a non-VBPL method named the diagram kind: %+v", obs.Findings)
	}
}

func TestRelationKindDisagreement_AmendmentDiagramCoversRepealReplaceAndSuspend(t *testing.T) {
	for _, kind := range []string{"repeals", "replaces", "suspends"} {
		obs := detectKinds(t, kindRelease(diagram("amends_supplements"), text(kind)))
		if len(obs.Findings) != 0 || obs.Excluded != 1 {
			t.Errorf("%s: %+v", kind, obs)
		}
	}
	for _, diagramKind := range []string{"repeals", "replaces", "suspends", "cites", "legal_basis"} {
		if obs := detectKinds(t, kindRelease(diagram(diagramKind), text("amends_supplements"))); len(obs.Findings) != 1 || obs.Excluded != 0 {
			t.Errorf("text amendment against %s: %+v", diagramKind, obs)
		}
	}
	if obs := detectKinds(t, kindRelease(diagram("cites"), diagram("legal_basis"), text("repeals"))); len(obs.Findings) != 1 {
		t.Errorf("citation kinds excused a repeal: %+v", obs.Findings)
	}
}

func TestRelationKindDisagreement_DocumentedSuspensionIsExcludedNotFired(t *testing.T) {
	obs := detectKinds(t, kindRelease(diagram("amends_supplements"), text("suspends")))
	if len(obs.Findings) != 0 || obs.Excluded != 1 {
		t.Fatalf("%+v", obs)
	}
	obs = detectKinds(t, kindRelease(diagram("amends_supplements"), diagram("cites"), text("suspends")))
	if len(obs.Findings) != 0 || obs.Excluded != 1 {
		t.Fatalf("with another diagram kind: %+v", obs)
	}
	obs = detectKinds(t, kindRelease(diagram("amends_supplements"), diagram("suspends"), text("suspends")))
	if len(obs.Findings) != 0 || obs.Excluded != 0 {
		t.Fatalf("matching kind is not an exclusion: %+v", obs)
	}
}

func TestRelationKindDisagreement_OnlyTextRuleRowsAreTheTextSide(t *testing.T) {
	reviewedText := pairRelation{Source: "vn:src", Target: "vn:target", Kind: "repeals", Method: "reviewed"}
	if obs := detectKinds(t, kindRelease(diagram("cites"), reviewedText)); len(obs.Findings) != 0 {
		t.Fatalf("reviewed row counted as text evidence: %+v", obs.Findings)
	}
	reviewedDiagram := pairRelation{Source: "vn:src", Target: "vn:target", Kind: "cites", Method: "reviewed"}
	if obs := detectKinds(t, kindRelease(reviewedDiagram, text("repeals"))); len(obs.Findings) != 0 {
		t.Fatalf("reviewed row counted as diagram evidence: %+v", obs.Findings)
	}
	if obs := detectKinds(t, kindRelease(diagram("unknown"), text("repeals"))); len(obs.Findings) != 0 {
		t.Fatalf("an unmapped diagram code named a kind: %+v", obs.Findings)
	}
}

func TestRelationKindDisagreement_OneFindingPerPairAndTextKind(t *testing.T) {
	other := pairRelation{Source: "vn:src", Target: "vn:other", Kind: "repeals", Method: "vbpl_diagram"}
	otherText := pairRelation{Source: "vn:src", Target: "vn:other", Kind: "replaces", Method: "text_rule/1"}
	rel := kindRelease(diagram("cites"), text("repeals"), text("amends_supplements"), other, otherText)
	rel.documents = append(rel.documents, releaseDocument{ID: 3, Key: "vn:other", HasProvisions: true})
	obs := detectKinds(t, rel)
	if len(obs.Findings) != 3 {
		t.Fatalf("%+v", obs.Findings)
	}
	file := mustCompute(t, computeInput{Runs: []detectorRun{run(detector.RelationKindDisagreement, 1, obs.ObservedCompletely, obs.Findings...)}})
	if len(file.Findings) != 3 {
		t.Fatalf("keys collide: %+v", file.Findings)
	}
}

func TestRelationKindDisagreement_FingerprintHoldsBothKindsNothingVolatile(t *testing.T) {
	a := detectKinds(t, kindRelease(diagram("cites"), diagram("implements"), text("repeals"))).Findings[0]
	b := detectKinds(t, kindRelease(diagram("implements"), diagram("cites"), text("repeals"))).Findings[0]
	fa, _ := hashValue(a.FingerprintInputs)
	fb, _ := hashValue(b.FingerprintInputs)
	if fa != fb {
		t.Fatal("fingerprint depends on row order")
	}
	c := detectKinds(t, kindRelease(diagram("cites"), text("repeals"))).Findings[0]
	if fc, _ := hashValue(c.FingerprintInputs); fc == fa {
		t.Fatal("fingerprint ignores the diagram kinds")
	}
	if strings.Join(a.FingerprintInputs, "|") != "repeals|cites,implements" {
		t.Fatalf("inputs %v", a.FingerprintInputs)
	}
}

func kindFile(t *testing.T, rel string, previous *File, obs Observation) File {
	t.Helper()
	r := run(detector.RelationKindDisagreement, 1, obs.ObservedCompletely, obs.Findings...)
	r.spec.Severity = detector.Review
	return mustCompute(t, computeInput{Rel: rel, Runs: []detectorRun{r}, Previous: previous})
}

func TestRelationKindDisagreement_LostInputCarriesThePreviousFinding(t *testing.T) {
	first := kindFile(t, "20990101_1", nil, detectKinds(t, kindRelease(diagram("repeals"), text("amends_supplements"))))
	if len(first.Findings) != 1 {
		t.Fatalf("first %+v", first.Findings)
	}
	for name, mutate := range map[string]func(*fakeRelease){
		"source baseline served": func(r *fakeRelease) { r.documents[0].Baseline = true },
		"source is chinhphu":     func(r *fakeRelease) { r.documents[0].Source = "chinhphu" },
		"source is consolidated": func(r *fakeRelease) { r.documents[0].Consolidated = true },
		"source has no rows":     func(r *fakeRelease) { r.documents[0].HasProvisions = false },
		"source blocked by a gap": func(r *fakeRelease) {
			r.gaps = map[string]map[string]bool{"vn:src": {"needs_ocr": true}}
		},
		"target not in release": func(r *fakeRelease) { r.documents = r.documents[:1] },
	} {
		t.Run(name, func(t *testing.T) {
			rel := kindRelease(diagram("repeals"))
			mutate(rel)
			second := kindFile(t, "20990102_1", &first, detectKinds(t, rel))
			if len(second.Gone) != 0 || len(second.Carried) != 1 {
				t.Fatalf("gone %v carried %v %+v", second.Gone, second.Carried, second.Findings)
			}
		})
	}
}

func TestRelationKindDisagreement_GapsThatRemoveTextRowsCarryThePreviousFinding(t *testing.T) {
	first := kindFile(t, "20990101_1", nil, detectKinds(t, kindRelease(diagram("repeals"), text("amends_supplements"))))
	for _, kind := range append([]string{"tree_headings_partial", "tree_headings_only"}, textRowGapKinds()...) {
		t.Run(kind, func(t *testing.T) {
			rel := kindRelease(diagram("repeals"))
			gaps := map[string]map[string]bool{"vn:src": {kind: true}}
			rel.gaps, rel.allGaps = gaps, gaps
			second := kindFile(t, "20990102_1", &first, detectKinds(t, rel))
			if len(second.Gone) != 0 || len(second.Carried) != 1 {
				t.Fatalf("gone %v carried %v", second.Gone, second.Carried)
			}
		})
	}
	rel := kindRelease(diagram("repeals"))
	gaps := map[string]map[string]bool{"vn:other": {"relation_text_unlinked_target": true}}
	rel.gaps, rel.allGaps = gaps, gaps
	if second := kindFile(t, "20990102_1", &first, detectKinds(t, rel)); len(second.Gone) != 1 {
		t.Fatalf("a gap on another source carried the finding: gone %v carried %v", second.Gone, second.Carried)
	}
}

func TestRelationKindDisagreement_ResolvedPairIsGone(t *testing.T) {
	first := kindFile(t, "20990101_1", nil, detectKinds(t, kindRelease(diagram("repeals"), text("amends_supplements"))))
	for name, rows := range map[string][]pairRelation{
		"text row gone":      {diagram("repeals")},
		"diagram now agrees": {diagram("repeals"), diagram("amends_supplements"), text("amends_supplements")},
	} {
		second := kindFile(t, "20990102_1", &first, detectKinds(t, kindRelease(rows...)))
		if len(second.Gone) != 1 || len(second.Carried) != 0 {
			t.Errorf("%s: gone %v carried %v", name, second.Gone, second.Carried)
		}
	}
}

func TestRelationKindDisagreement_ObservedCountsDocuments(t *testing.T) {
	rel := kindRelease(diagram("repeals"), text("amends_supplements"))
	rel.documents = append(rel.documents, releaseDocument{ID: 3, Key: "vn:base", Baseline: true, HasProvisions: true}, releaseDocument{ID: 4, Key: "vn:empty"})
	obs := detectKinds(t, rel)
	if obs.Observed != 2 || obs.NotObservable != 2 {
		t.Fatalf("observed %d not observable %d", obs.Observed, obs.NotObservable)
	}
}
