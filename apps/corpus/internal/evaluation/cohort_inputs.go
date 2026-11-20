package evaluation

type cohortInput struct {
	c       Case
	cohorts []string
	legacy  bool
}

func (i cohortInput) inForce() bool { return i.cohorts[0] != "development_period" }

func developmentCohort(c Case) string {
	for _, tag := range c.Tags {
		if tag == "historical" {
			return "development_period"
		}
	}
	return "development"
}

func cohortInputs(s Suite, r *Report) []cohortInput {
	var out []cohortInput
	for _, c := range s.Legacy {
		out = append(out, cohortInput{c, []string{"legacy", "old_product"}, true})
	}
	counts := map[string]int{}
	for _, c := range s.Development {
		cohort := developmentCohort(c)
		counts[cohort]++
		out = append(out, cohortInput{c, []string{cohort}, false})
	}
	for _, cohort := range []string{"development", "development_period"} {
		r.Cohorts[cohort] = Cohort{}
		if counts[cohort] > 0 {
			delete(r.NotRun, cohort)
		} else {
			r.NotRun[cohort] = "not_run"
		}
	}
	return out
}

func reportCohorts(r *Report) []string {
	var out []string
	for _, name := range []string{"legacy", "old_product", "development", "development_period"} {
		if _, ok := r.Cohorts[name]; ok {
			out = append(out, name)
		}
	}
	return out
}
