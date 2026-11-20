package evaluation

import (
	"encoding/json"
	"fmt"
)

func decodePreviousReport(data []byte) (Report, error) {
	var legacy any
	if err := json.Unmarshal(data, &legacy); err != nil {
		return Report{}, fmt.Errorf("decode previous report: %w", err)
	}
	if err := visitObjects(legacy, rejectRemovedOverrides); err != nil {
		return Report{}, err
	}
	var report Report
	if err := json.Unmarshal(data, &report); err != nil {
		return Report{}, fmt.Errorf("decode previous report: %w", err)
	}
	if report.Version >= 2 {
		if err := validatePreviousMRR(legacy); err != nil {
			return Report{}, err
		}
	}
	return report, nil
}

func visitObjects(value any, visit func(map[string]any) error) error {
	switch fields := value.(type) {
	case map[string]any:
		if err := visit(fields); err != nil {
			return err
		}
		for _, child := range fields {
			if err := visitObjects(child, visit); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range fields {
			if err := visitObjects(child, visit); err != nil {
				return err
			}
		}
	}
	return nil
}

func rejectRemovedOverrides(fields map[string]any) error {
	if marker, present := fields["merge_siblings"]; present && marker != nil {
		enabled, ok := marker.(bool)
		if !ok {
			return fmt.Errorf("invalid previous merge_siblings marker")
		}
		if enabled {
			return fmt.Errorf("incompatible previous sibling merge override: merge_siblings:true is a research override")
		}
	}
	if policy, present := fields["historical_policy"]; present && policy != nil {
		return fmt.Errorf("incompatible previous historical_policy override: the research override was removed")
	}
	return nil
}
