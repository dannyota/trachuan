package evidence

import (
	"context"
	"errors"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/detector"
)

type stubDetector struct {
	id  string
	obs Observation
	err error
}

func (d stubDetector) Spec() detector.Detector { return specOf(d.id) }

func (d stubDetector) Detect(context.Context, Inputs) (Observation, error) { return d.obs, d.err }

func TestCountFindings_CountsPerDetectorAndNamesFailures(t *testing.T) {
	counts := CountFindings(context.Background(), Inputs{}, []Detector{
		stubDetector{id: detector.UnmatchedOperativeClaim, obs: Observation{Observed: 5, NotObservable: 1, Findings: []Fired{{Severity: "review"}, {Severity: "info"}, {Severity: "review"}}}},
		stubDetector{id: detector.RawFieldNotNormalized, obs: Observation{Observed: 2, Findings: []Fired{{Discriminator: "number"}}}},
		stubDetector{id: detector.WaivedDocumentWithoutRelations, err: errors.New("extract artifacts are not available")},
	})
	if len(counts) != 3 {
		t.Fatalf("counts %+v", counts)
	}
	byID := map[string]Count{}
	for _, c := range counts {
		byID[c.Detector] = c
	}
	claim := byID[detector.UnmatchedOperativeClaim]
	if claim.Findings != 3 || claim.Observed != 5 || claim.NotObservable != 1 || claim.Classes["review"] != 2 || claim.Classes["info"] != 1 {
		t.Fatalf("claim %+v", claim)
	}
	if raw := byID[detector.RawFieldNotNormalized]; raw.Findings != 1 || raw.Classes != nil {
		t.Fatalf("raw %+v", raw)
	}
	if w := byID[detector.WaivedDocumentWithoutRelations]; w.NotRun != "extract artifacts are not available" || w.Findings != 0 {
		t.Fatalf("waived %+v", w)
	}
}

func TestCountFindings_OrderedByDetectorID(t *testing.T) {
	counts := CountFindings(context.Background(), Inputs{}, []Detector{stubDetector{id: detector.WaivedDocumentWithoutRelations}, stubDetector{id: detector.PartialWithoutProvisionEffect}})
	if counts[0].Detector != detector.PartialWithoutProvisionEffect {
		t.Fatalf("counts %+v", counts)
	}
}
