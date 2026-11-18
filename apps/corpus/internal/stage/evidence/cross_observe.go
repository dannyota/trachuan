package evidence

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

	"danny.vn/trachuan/apps/corpus/internal/detector"
	"danny.vn/trachuan/apps/corpus/internal/stage/relate"
)

func gapState(gaps map[string]bool) (blocked, headings bool) {
	for kind := range gaps {
		if relate.BlockingGap(kind) {
			blocked = true
		} else if relate.IncompleteGap(kind) {
			headings = true
		}
	}
	return blocked, headings
}

func neverRead(d releaseDocument) bool { return d.Source == "chinhphu" || d.Consolidated }

func sha(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

type coverage struct {
	documents map[string]bool
	observed  int
	missing   int
}

func newCoverage() *coverage { return &coverage{documents: map[string]bool{}} }

func (c *coverage) add(d releaseDocument) {
	if d.Baseline {
		c.missing++
		return
	}
	c.observed++
	c.documents[d.Key] = true
}

func (c *coverage) done(findings []Fired) Observation {
	return Observation{Findings: findings, Observed: c.observed, NotObservable: c.missing, ObservedCompletely: func(s Subject) bool {
		return len(s.Fields) > 0 && c.documents[s.Fields[0]]
	}}
}

func documentFired(key, discriminator string, inputs ...string) Fired {
	return Fired{Subject: Subject{Type: detector.SubjectDocument, Fields: []string{key}}, Discriminator: discriminator, FingerprintInputs: inputs}
}

func scanDocuments(ctx context.Context, rel releaseData, visit func(releaseDocument) ([]Fired, error)) (Observation, error) {
	cov := newCoverage()
	var findings []Fired
	err := rel.Documents(ctx, func(d releaseDocument) error {
		cov.add(d)
		fired, err := visit(d)
		findings = append(findings, fired...)
		return err
	})
	if err != nil {
		return Observation{}, err
	}
	return cov.done(findings), nil
}
