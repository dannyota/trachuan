package evaluation

import (
	"encoding/json"
	"fmt"
)

func decodePreviousReport(data []byte) (Report, error) {
	// Read legacy markers separately so they never become writable report fields.
	var legacy any
	if err := json.Unmarshal(data, &legacy); err != nil {
		return Report{}, fmt.Errorf("decode previous report: %w", err)
	}
	if err := rejectLegacySiblingMerge(legacy); err != nil {
		return Report{}, err
	}
	var report Report
	if err := json.Unmarshal(data, &report); err != nil {
		return Report{}, fmt.Errorf("decode previous report: %w", err)
	}
	return report, nil
}

func rejectLegacySiblingMerge(value any) error {
	switch fields := value.(type) {
	case map[string]any:
		if marker, present := fields["merge_siblings"]; present && marker != nil {
			enabled, ok := marker.(bool)
			if !ok {
				return fmt.Errorf("invalid previous merge_siblings marker")
			}
			if enabled {
				return fmt.Errorf("incompatible previous sibling merge override: merge_siblings:true is a research override")
			}
		}
		for _, child := range fields {
			if err := rejectLegacySiblingMerge(child); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range fields {
			if err := rejectLegacySiblingMerge(child); err != nil {
				return err
			}
		}
	}
	return nil
}
