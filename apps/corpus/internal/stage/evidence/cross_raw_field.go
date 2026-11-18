package evidence

import (
	"context"
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/detector"
	"golang.org/x/text/unicode/norm"
)

type rawFieldDetector struct{}

func (rawFieldDetector) Spec() detector.Detector { return specOf(detector.RawFieldNotNormalized) }

func (rawFieldDetector) Detect(ctx context.Context, in Inputs) (Observation, error) {
	return detectRawFields(ctx, sqlRelease{in.Release})
}

func normalizedField(value string) string { return norm.NFC.String(strings.TrimSpace(value)) }

func detectRawFields(ctx context.Context, rel releaseData) (Observation, error) {
	return scanDocuments(ctx, rel, func(d releaseDocument) ([]Fired, error) {
		var out []Fired
		for _, f := range []struct{ name, value string }{{"number", d.Number}, {"title", d.Title}} {
			if want := normalizedField(f.value); want != f.value {
				out = append(out, documentFired(d.Key, f.name, f.name, sha(f.value), sha(want)))
			}
		}
		return out, nil
	})
}
