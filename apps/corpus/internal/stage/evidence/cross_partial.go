package evidence

import (
	"context"

	"danny.vn/trachuan/apps/corpus/internal/detector"
)

type partialDetector struct{}

func (partialDetector) Spec() detector.Detector {
	return specOf(detector.PartialWithoutProvisionEffect)
}

func (partialDetector) Detect(ctx context.Context, in Inputs) (Observation, error) {
	return detectPartial(ctx, sqlRelease{in.Release})
}

func detectPartial(ctx context.Context, rel releaseData) (Observation, error) {
	return scanDocuments(ctx, rel, func(d releaseDocument) ([]Fired, error) {
		if d.StatusClass != "partial" || d.ProvisionTargeted {
			return nil, nil
		}
		return []Fired{documentFired(d.Key, "", d.StatusClass, "0")}, nil
	})
}
