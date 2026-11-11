package build

import (
	"encoding/json"
	"fmt"
	"sort"

	"danny.vn/trachuan/apps/corpus/internal/source"
)

type heldCorrection struct {
	entry         source.Correction
	missing, kind string
	targetMissing bool
}

func (l *loader) correctionGap(key string) string {
	for _, g := range l.gaps {
		if g.ItemKey == key && g.Stage == "fetch" && g.Kind == "source_unavailable" {
			return g.Kind
		}
	}
	return ""
}

func (l *loader) validateCorrectionGaps(corrections []source.Correction, docs map[string]correctionDocument) (map[string]source.Correction, []heldCorrection, error) {
	entries := make(map[string]source.Correction, len(corrections))
	seen := map[string]bool{}
	var held []heldCorrection
	for _, c := range corrections {
		if seen[c.Key] {
			return nil, nil, fmt.Errorf("correction %q: duplicate key", c.Key)
		}
		seen[c.Key] = true
		local := make(map[string]correctionDocument, 2)
		if d, ok := docs[c.Key]; ok {
			local[c.Key] = d
		}
		if d, ok := docs[c.Evidence.Key]; ok {
			local[c.Evidence.Key] = d
		}
		var h heldCorrection
		h.entry = c
		if _, ok := local[c.Key]; !ok {
			if l.members[c.Key].Disposition == "active" {
				h.kind = l.correctionGap(c.Key)
			}
			if h.kind == "" {
				return nil, nil, fmt.Errorf("correction %q: document absent", c.Key)
			}
			h.missing = c.Key
			h.targetMissing = true
			local[c.Key] = correctionDocument{number: c.Number}
		}
		evidenceKey := c.Key
		if c.Evidence.Key != "" {
			evidenceKey = c.Evidence.Key
		}
		if c.Evidence.Anchor != "" {
			d, ok := local[evidenceKey]
			if !ok {
				kind := ""
				if l.members[evidenceKey].Disposition == "active" {
					kind = l.correctionGap(evidenceKey)
				}
				if kind == "" {
					return nil, nil, fmt.Errorf("correction %q: evidence document %q absent", c.Key, evidenceKey)
				}
				if h.kind == "" {
					h.missing, h.kind = evidenceKey, kind
				}
				d = correctionDocument{}
			}
			if !ok {
				d.provisions = map[string]string{c.Evidence.Anchor: c.Evidence.Quote}
			} else if h.targetMissing && evidenceKey == c.Key {
				d.provisions = map[string]string{c.Evidence.Anchor: c.Evidence.Quote}
			}
			local[evidenceKey] = d
		}
		validated, err := validateCorrections([]source.Correction{c}, local)
		if err != nil {
			return nil, nil, err
		}
		if h.kind != "" {
			held = append(held, h)
		} else {
			entries[c.Key] = validated[c.Key]
		}
	}
	return entries, held, nil
}

func (l *loader) recordCorrectionGaps(result *validityResult, held []heldCorrection) error {
	ordered := append([]heldCorrection(nil), held...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].entry.Key < ordered[j].entry.Key })
	for _, h := range ordered {
		detail, err := json.Marshal(struct {
			EntryKey   string `json:"entry_key"`
			MissingKey string `json:"missing_document_key"`
			GapKind    string `json:"gap_kind"`
		}{h.entry.Key, h.missing, h.kind})
		if err != nil {
			return fmt.Errorf("encode correction gap: %w", err)
		}
		l.addGap("build", h.entry.Key, "correction_evidence_unavailable", string(detail))
		result.reviews = append(result.reviews, validityReview{kind: "correction_evidence_unavailable", key: h.entry.Key, number: h.entry.Number, targetKey: h.missing, detail: string(detail)})
		result.counts.review["correction_evidence_unavailable"]++
		if h.targetMissing {
			continue
		}
		o, ok := result.documents[h.entry.Key]
		if !ok {
			return fmt.Errorf("correction %q: target missing", h.entry.Key)
		}
		before := o.values
		o.values.Conflict = true
		if len(o.history) == 0 {
			o.history = append(o.history, validityDecision{layer: "source", version: "validity/2", before: before, after: before})
		}
		o.history = append(o.history, validityDecision{layer: "manual", version: "correction_unverified", before: before, after: o.values, actor: h.missing})
		if len(o.history) > 8 {
			return fmt.Errorf("validity %s: decision history exceeds 8", h.entry.Key)
		}
		result.documents[h.entry.Key] = o
	}
	return nil
}
