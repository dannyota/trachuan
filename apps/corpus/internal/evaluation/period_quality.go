package evaluation

import "danny.vn/trachuan/internal/retrieval"

func periodRecords(r, previous *Report) []NonRegression {
	if r.Cohorts["development_period"].CaseCount == 0 {
		return nil
	}
	out := []NonRegression{}
	for _, mode := range []retrieval.Mode{"exact", "ann"} {
		for _, spec := range []struct {
			name  string
			limit int
		}{{"recall", 20}, {"recall", 8}, {"mrr", 8}} {
			m := findQualityMetric(r, "development_period", mode, spec.name, spec.limit)
			var old *float64
			if previous != nil {
				old = findQualityMetric(previous, "development_period", mode, spec.name, spec.limit).Value
			}
			out = append(out, NonRegression{Mode: mode, Arms: "hybrid", Limit: spec.limit, Name: spec.name, Value: m.Value, PreviousValue: old, Status: "reported", ValidityOverrides: r.ValidityOverrides, CapOverride: r.CapOverride, ArticleCapOverride: r.ArticleCapOverride, Reason: m.Reason, MappingIncompleteCount: m.MappingIncompleteCount})
		}
	}
	return out
}
