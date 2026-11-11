package evaluation

type cohortInput struct {
	c       Case
	cohorts []string
	legacy  bool
}

func cohortInputs(s Suite, r *Report) []cohortInput {
	var out []cohortInput
	for _, c := range s.Legacy {
		out = append(out, cohortInput{c, []string{"legacy", "old_product"}, true})
	}
	for _, c := range s.Development {
		out = append(out, cohortInput{c, []string{"development"}, false})
	}
	r.Cohorts["development"] = Cohort{}
	if len(s.Development) > 0 {
		delete(r.NotRun, "development")
	}
	return out
}

func reportCohorts(r *Report) []string {
	var out []string
	for _, name := range []string{"legacy", "old_product", "development"} {
		if _, ok := r.Cohorts[name]; ok {
			out = append(out, name)
		}
	}
	return out
}
