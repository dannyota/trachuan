package extract

import (
	"context"
	"fmt"
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/source"
)

// HeadingCorrectionCheck reports whether a switching evidence document retains
// a manual correction's exact quote at its anchor.
type HeadingCorrectionCheck struct {
	Key         string `json:"key"`
	EvidenceKey string `json:"evidence_key"`
	Anchor      string `json:"anchor"`
	QuoteFound  bool   `json:"quote_found"`
}

// CheckHeadingCorrections checks corrections against the run's fetch bundles.
// Documents absent from bundles do not belong to this run.
func CheckHeadingCorrections(ctx context.Context, entries []source.Correction, bundles map[string]string) ([]HeadingCorrectionCheck, error) {
	checks := make([]HeadingCorrectionCheck, 0)
	type evidenceText struct {
		switched bool
		anchors  map[string]string
	}
	cached := make(map[string]evidenceText)
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		key := entry.Evidence.Key
		if key == "" {
			key = entry.Key
		}
		dir, found := bundles[key]
		if !found {
			continue
		}
		evidence, loaded := cached[key]
		if !loaded {
			selected, _, err := choose(ctx, dir, key)
			if err != nil {
				return nil, fmt.Errorf("select correction evidence %q: %w", key, err)
			}
			evidence.anchors = make(map[string]string)
			evidence.switched = selected.tree != nil && selected.tree.SetAside
			if evidence.switched {
				_, _, record := parseTranscription(selected.data, selected.name, selected.hash)
				for _, p := range record.Provisions {
					evidence.anchors[p.Anchor] = p.Text
				}
			}
			cached[key] = evidence
		}
		if !evidence.switched {
			continue
		}
		text, found := evidence.anchors[entry.Evidence.Anchor]
		quoteFound := found && strings.TrimSpace(entry.Evidence.Quote) != "" && strings.Contains(text, entry.Evidence.Quote)
		checks = append(checks, HeadingCorrectionCheck{Key: entry.Key, EvidenceKey: key, Anchor: entry.Evidence.Anchor, QuoteFound: quoteFound})
	}
	return checks, nil
}
