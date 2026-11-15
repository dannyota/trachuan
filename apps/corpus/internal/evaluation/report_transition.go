package evaluation

import "fmt"

func validateReportTransition(current, previous *Report) error {
	if previous != nil && current.Version == 1 && previous.Version == 2 {
		return fmt.Errorf("report version cannot downgrade from 2 to 1")
	}
	counts, err := reportCaseCounts(current)
	if err != nil {
		return err
	}
	if previous == nil {
		return nil
	}
	prior, err := reportCaseCounts(previous)
	if err != nil {
		return fmt.Errorf("previous report cohorts: %w", err)
	}
	for name, count := range prior {
		if count > 0 && counts[name] == 0 {
			return fmt.Errorf("populated previous cohort %s is empty or absent", name)
		}
	}
	return nil
}

func reportCaseCounts(report *Report) (map[string]int, error) {
	ids := map[string]map[string]bool{}
	for name := range report.Cohorts {
		ids[name] = map[string]bool{}
	}
	for _, c := range report.Cases {
		cases, ok := ids[c.Cohort]
		if !ok || c.CaseID == "" {
			return nil, fmt.Errorf("case has an unknown cohort or empty identity")
		}
		cases[c.CaseID] = true
	}
	if cases, ok := ids["old_product"]; ok {
		for _, rejection := range report.Rejections {
			if rejection.CaseID == "" {
				return nil, fmt.Errorf("rejection has an empty identity")
			}
			cases[rejection.CaseID] = true
		}
	}
	counts := map[string]int{}
	for name, cases := range ids {
		counts[name] = len(cases)
		if report.Cohorts[name].CaseCount != len(cases) {
			return nil, fmt.Errorf("cohort %s case_count differs from case records", name)
		}
	}
	return counts, nil
}
