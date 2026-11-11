package build

import (
	"encoding/json"
	"fmt"
	"sort"

	"danny.vn/trachuan/apps/corpus/internal/source"
)

type canonicalCorrection struct {
	Key      string                      `json:"key"`
	Number   string                      `json:"number"`
	Set      canonicalCorrectionSet      `json:"set"`
	Eligible *bool                       `json:"eligible,omitempty"`
	Hold     bool                        `json:"hold,omitempty"`
	Reason   string                      `json:"reason"`
	Evidence canonicalCorrectionEvidence `json:"evidence"`
	Reviewed string                      `json:"reviewed"`
}
type canonicalCorrectionSet struct {
	StatusClass *string `json:"status_class,omitempty"`
	EffFrom     *string `json:"eff_from,omitempty"`
	EffTo       *string `json:"eff_to,omitempty"`
}
type canonicalCorrectionEvidence struct {
	Key    string `json:"key,omitempty"`
	Anchor string `json:"anchor,omitempty"`
	URL    string `json:"url,omitempty"`
	Quote  string `json:"quote"`
}

func canonicalCorrections(entries []source.Correction) ([]byte, error) {
	ordered := append([]source.Correction{}, entries...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Key < ordered[j].Key })
	result := make([]canonicalCorrection, 0, len(ordered))
	for i, e := range ordered {
		if i > 0 && ordered[i-1].Key == e.Key {
			return nil, fmt.Errorf("correction %q: duplicate key", e.Key)
		}
		result = append(result, canonicalCorrection{Key: e.Key, Number: e.Number, Set: canonicalCorrectionSet{e.Set.StatusClass, e.Set.EffFrom, e.Set.EffTo}, Eligible: e.Eligible, Hold: e.Hold, Reason: e.Reason, Evidence: canonicalCorrectionEvidence{Key: e.Evidence.Key, Anchor: e.Evidence.Anchor, URL: e.Evidence.URL, Quote: e.Evidence.Quote}, Reviewed: e.Reviewed})
	}
	data, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("encode corrections: %w", err)
	}
	return data, nil
}
