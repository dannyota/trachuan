package build

import (
	"fmt"
	"strings"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/internal/retrieval"
)

type correctionDocument struct {
	number     string
	provisions map[string]string
	from, to   *time.Time
}

func validateCorrections(entries []source.Correction, docs map[string]correctionDocument) (map[string]source.Correction, error) {
	result := make(map[string]source.Correction, len(entries))
	for _, entry := range entries {
		doc, ok := docs[entry.Key]
		if !ok {
			return nil, fmt.Errorf("correction %q: document absent", entry.Key)
		}
		if _, duplicate := result[entry.Key]; duplicate {
			return nil, fmt.Errorf("correction %q: duplicate key", entry.Key)
		}
		if entry.Number == "" || retrieval.NormalizeNumber(entry.Number) != retrieval.NormalizeNumber(doc.number) {
			return nil, fmt.Errorf("correction %q: number mismatch", entry.Key)
		}
		if strings.TrimSpace(entry.Reason) == "" || strings.TrimSpace(entry.Evidence.Quote) == "" {
			return nil, fmt.Errorf("correction %q: missing reason or quote", entry.Key)
		}
		if _, err := time.Parse("2006-01-02", entry.Reviewed); err != nil {
			return nil, fmt.Errorf("correction %q: invalid review date: %w", entry.Key, err)
		}
		if (entry.Evidence.Anchor == "") == (entry.Evidence.URL == "") || (entry.Evidence.Key != "" && entry.Evidence.Anchor == "") {
			return nil, fmt.Errorf("correction %q: invalid evidence location", entry.Key)
		}
		evidenceDoc := doc
		if entry.Evidence.Key != "" {
			var found bool
			evidenceDoc, found = docs[entry.Evidence.Key]
			if !found {
				return nil, fmt.Errorf("correction %q: evidence document %q absent", entry.Key, entry.Evidence.Key)
			}
		}
		if entry.Evidence.Anchor != "" {
			text, found := evidenceDoc.provisions[entry.Evidence.Anchor]
			if !found || !strings.Contains(text, entry.Evidence.Quote) {
				return nil, fmt.Errorf("correction %q: evidence not found", entry.Key)
			}
		}
		set := entry.Set
		if set.StatusClass == nil && set.EffFrom == nil && set.EffTo == nil && entry.Eligible == nil && !entry.Hold {
			return nil, fmt.Errorf("correction %q: no changes", entry.Key)
		}
		if set.StatusClass != nil {
			switch *set.StatusClass {
			case "in_force", "partial", "not_yet", "expired", "suspended", "unknown":
			default:
				return nil, fmt.Errorf("correction %q: invalid status class", entry.Key)
			}
		}
		for _, value := range []*string{set.EffFrom, set.EffTo} {
			if value != nil {
				if _, err := time.Parse("2006-01-02", *value); err != nil {
					return nil, fmt.Errorf("correction %q: invalid date: %w", entry.Key, err)
				}
			}
		}
		result[entry.Key] = entry
	}
	return result, nil
}

func checkCorrections(entries []source.Correction, docs map[string]correctionDocument) []error {
	var failures []error
	seen := map[string]bool{}
	for _, entry := range entries {
		if seen[entry.Key] {
			failures = append(failures, fmt.Errorf("correction %q: duplicate key", entry.Key))
			continue
		}
		seen[entry.Key] = true
		if _, err := validateCorrections([]source.Correction{entry}, docs); err != nil {
			failures = append(failures, err)
			continue
		}
		doc := docs[entry.Key]
		before := validityValues{}
		if doc.from != nil {
			from := doc.from.Format("2006-01-02")
			before.From = &from
		}
		if doc.to != nil {
			to := doc.to.Format("2006-01-02")
			before.To = &to
		}
		after, _ := applyCorrection(before, entry, false)
		if err := validateCorrectionInterval(entry.Key, after); err != nil {
			failures = append(failures, err)
		}
	}
	return failures
}

func validateCorrectionInterval(key string, values validityValues) error {
	if values.From != nil && values.To != nil && *values.To < *values.From {
		return fmt.Errorf("validity %s: invalid interval", key)
	}
	return nil
}

func addCorrectionEvidence(docs map[string]correctionDocument, labels map[int64]string, key string, id int64, parent *int64, label, anchor, text string) {
	name := label
	if parent != nil && labels[*parent] != "" {
		name = labels[*parent] + " " + strings.ToLower(label)
	}
	labels[id] = name
	doc := docs[key]
	doc.provisions[anchor] = text
	if name != "" {
		doc.provisions[name] = text
	}
	docs[key] = doc
}
