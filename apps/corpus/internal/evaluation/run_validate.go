package evaluation

import (
	"bytes"
	"encoding/json"
	"fmt"
)

func rejectionSet(s Suite) map[string]bool {
	out := map[string]bool{}
	for _, id := range s.Rejections {
		out[id] = true
	}
	return out
}
func validateSuite(s Suite) error {
	b, err := json.Marshal(s.Legacy)
	if err != nil {
		return fmt.Errorf("encode suite: %w", err)
	}
	if _, err = LoadLegacy(bytes.NewReader(b)); err != nil {
		return fmt.Errorf("validate suite: %w", err)
	}
	if (s.Map.Version != 1 && s.Map.Version != 2) || s.Map.LegacySHA256 != s.Hashes.Legacy {
		return fmt.Errorf("invalid map version or legacy hash")
	}
	ids := map[string]bool{}
	for _, c := range s.Legacy {
		ids[c.ID] = true
	}
	development, err := json.Marshal(s.Development)
	if err != nil {
		return fmt.Errorf("encode development: %w", err)
	}
	if s.Development != nil {
		if _, err = LoadDevelopment(bytes.NewReader(development)); err != nil {
			return fmt.Errorf("validate development: %w", err)
		}
	}
	seen := map[string]bool{}
	if len(s.Rejections) != 3 {
		return fmt.Errorf("expected three rejection checks")
	}
	for _, id := range s.Rejections {
		if !ids[id] || seen[id] {
			return fmt.Errorf("invalid rejection ID")
		}
		seen[id] = true
	}
	for _, c := range s.Development {
		if ids[c.ID] {
			return fmt.Errorf("duplicate case ID across cohorts")
		}
		ids[c.ID] = true
	}
	return nil
}
