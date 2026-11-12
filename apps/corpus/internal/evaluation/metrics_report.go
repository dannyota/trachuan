package evaluation

import (
	"sort"

	"danny.vn/trachuan/internal/retrieval"
)

type presentedKey struct {
	cohort string
	mode   retrieval.Mode
	detail retrieval.Detail
	limit  int
}

type presentedSamples struct {
	cases, expected, found int
	sizes                  []float64
}

func compactHits(hits []retrieval.Hit) []retrieval.Hit {
	out := append([]retrieval.Hit(nil), hits...)
	for i := range out {
		out[i].Fragments = nil
		out[i].Relations = nil
		out[i].Complete = false
	}
	return out
}

func finishPresented(r *Report, all map[presentedKey]*presentedSamples, modes []retrieval.Mode) {
	for _, cohort := range reportCohorts(r) {
		for _, mode := range modes {
			for _, detail := range []retrieval.Detail{retrieval.DetailCompact, retrieval.DetailFull} {
				for _, limit := range []int{8, 20} {
					s := all[presentedKey{cohort, mode, detail, limit}]
					if s == nil {
						continue
					}
					sort.Float64s(s.sizes)
					r.Presented = append(r.Presented, PresentedSummary{ValidityOverrides: r.ValidityOverrides, CapOverride: r.CapOverride, Cohort: cohort, Mode: mode, Detail: detail, Limit: limit, Cases: s.cases, Expected: s.expected, Found: s.found, SizeP50: Quantile(s.sizes, .5), SizeP95: Quantile(s.sizes, .95)})
				}
			}
		}
	}
	setCompactSizeGate(r)
}

func setCompactSizeGate(r *Report) {
	var gate CompactSizeGate
	for _, s := range r.Presented {
		if s.Cohort != "old_product" || s.Mode != "exact" {
			continue
		}
		if s.Detail == retrieval.DetailFull && s.Limit == 8 {
			gate.FullP95At8 = s.SizeP95
		}
		if s.Detail == retrieval.DetailCompact && s.Limit == 20 {
			gate.CompactP95At20 = s.SizeP95
		}
	}
	gate.Pass = gate.FullP95At8 != nil && gate.CompactP95At20 != nil && *gate.CompactP95At20 <= *gate.FullP95At8
	r.CompactSizeGate = gate
}

func finishReport(r *Report, all map[metricKey]*Metrics, modes []retrieval.Mode) {
	for _, cohort := range reportCohorts(r) {
		stats := r.Cohorts[cohort]
		for _, mode := range modes {
			for _, arms := range []retrieval.ArmMode{"hybrid", "dense", "lexical"} {
				for _, limit := range []int{8, 20} {
					m := all[metricKey{cohort, mode, arms, limit}]
					if m == nil {
						continue
					}
					matched, total, positive := 0, 0, 0
					macro, mrr := 0.0, 0.0
					for _, c := range m.Cases {
						if c.Negative {
							continue
						}
						positive++
						n, best := 0, 0
						for _, rank := range c.Ranks {
							if rank > 0 && rank <= limit {
								n++
							}
							if rank > 0 && rank <= 8 && (best == 0 || rank < best) {
								best = rank
							}
						}
						matched += n
						total += len(c.Ranks)
						if len(c.Ranks) > 0 {
							macro += float64(n) / float64(len(c.Ranks))
						}
						if best > 0 {
							mrr += 1 / float64(best)
						}
					}
					add := func(name string, num float64, den int, conf *Confusion) {
						var value *float64
						if den > 0 {
							v := num / float64(den)
							value = &v
						}
						metric := Metric{Mode: mode, Arms: arms, Limit: limit, Name: name, Numerator: num, Denominator: den, Value: value, Confusion: conf, ValidityOverrides: r.ValidityOverrides, CapOverride: r.CapOverride}
						stats.Metrics = append(stats.Metrics, metric)
					}
					add("recall", float64(matched), total, nil)
					add("macro_recall", macro, positive, nil)
					if limit == 8 {
						add("mrr", mrr, positive, nil)
						details := m.PrecisionCounts()
						if cohort == "legacy" {
							current, total := 0, 0
							for _, c := range m.Cases {
								hs := c.Response.Hits
								end := len(hs)
								for end > 0 && !isCurrent(hs[end-1]) {
									end--
								}
								if end > 0 {
									hs = hs[:end]
								}
								for _, h := range hs {
									total++
									if isCurrent(h) {
										current++
									}
								}
							}
							add("current_law_precision", float64(current), total, nil)
						} else {
							add("current_law_precision", float64(details.PrimaryCurrent), details.PrimaryTotal, nil)
						}
						add("unknown_validity", float64(details.Unknown), details.PrimaryTotal, nil)
						add("contradictory_validity", float64(details.Contradictory), details.PrimaryTotal, nil)
						add("non_current_group_precision", float64(details.NonCurrentCurrent), details.NonCurrentTotal, nil)
					}
					conf := m.Abstention()
					add("abstention", float64(conf.TP+conf.TN), conf.TP+conf.TN+conf.FP+conf.FN, &conf)
					add("negative_recall", float64(conf.TP), conf.TP+conf.FN, nil)
					add("positive_answer_rate", float64(conf.TN), conf.TN+conf.FP, nil)
					balanced := Metric{Mode: mode, Arms: arms, Limit: limit, Name: "abstention_balanced_accuracy", Value: conf.BalancedAccuracy, Confusion: &conf, ValidityOverrides: r.ValidityOverrides, CapOverride: r.CapOverride}
					if conf.BalancedAccuracy != nil {
						balanced.Numerator = *conf.NegativeRecall + *conf.PositiveAnswerRate
						balanced.Denominator = 2
					}
					stats.Metrics = append(stats.Metrics, balanced)
					sizes := m.EvidenceCharacters()
					sort.Float64s(sizes)
					r.EvidenceCharacters = append(r.EvidenceCharacters, SizeSummary{ValidityOverrides: r.ValidityOverrides, CapOverride: r.CapOverride, Cohort: cohort, Mode: mode, Arms: arms, Limit: limit, N: len(sizes), P50: Quantile(sizes, .5), P95: Quantile(sizes, .95)})
				}
			}
		}
		r.Cohorts[cohort] = stats
	}
}

func markMappingUnavailable(r *Report) {
	markCohortMappingUnavailable(r, "old_product", 0)
}

func markCohortMappingUnavailable(r *Report, cohort string, count int) {
	stats := r.Cohorts[cohort]
	for i := range stats.Metrics {
		m := &stats.Metrics[i]
		if m.Name == "recall" || m.Name == "macro_recall" || m.Name == "mrr" {
			if m.Value != nil && count > 0 {
				m.Reason, m.MappingIncompleteCount = "mapping_incomplete", count
			}
			m.Value = nil
		}
	}
	r.Cohorts[cohort] = stats
}

func finishMappingReport(r *Report, inputs []cohortInput, rejects map[string]bool, mapping ValidationReport) {
	r.MappingIncomplete = []MappingIncomplete{}
	counts := map[string]int{}
	for _, input := range inputs {
		if rejects[input.c.ID] || len(input.c.ExpectedCitations) == 0 {
			continue
		}
		for _, cohort := range input.cohorts {
			if cohort == "legacy" {
				continue
			}
			found := false
			for _, issue := range mapping[input.c.ID] {
				if issue.Kind != "mapping_incomplete" {
					continue
				}
				found = true
				r.MappingIncomplete = append(r.MappingIncomplete, MappingIncomplete{input.c.ID, cohort, issue.CitationIndex, issue.Detail})
			}
			if found {
				counts[cohort]++
			}
		}
	}
	sort.Slice(r.MappingIncomplete, func(i, j int) bool {
		a, b := r.MappingIncomplete[i], r.MappingIncomplete[j]
		if a.Cohort != b.Cohort {
			return a.Cohort < b.Cohort
		}
		if a.CaseID != b.CaseID {
			return a.CaseID < b.CaseID
		}
		if a.CitationIndex != b.CitationIndex {
			return a.CitationIndex < b.CitationIndex
		}
		return a.Detail < b.Detail
	})
	for cohort, count := range counts {
		markCohortMappingUnavailable(r, cohort, count)
	}
}

func finishQuality(r *Report, previous *Report) {
	finishProductQuality(r, previous)
	old := r.Cohorts["old_product"]
	old.Floors, old.Targets, old.NonRegression = r.Floors, r.Targets, r.NonRegression
	r.Cohorts["old_product"] = old
	development := r.Cohorts["development"]
	current := Report{Cohorts: map[string]Cohort{"old_product": development}, Rejections: r.Rejections, ValidityOverrides: r.ValidityOverrides, CapOverride: r.CapOverride}
	var prior *Report
	if previous != nil && previous.Cohorts["development"].CaseCount > 0 {
		prior = &Report{Cohorts: map[string]Cohort{"old_product": previous.Cohorts["development"]}}
	}
	finishProductQuality(&current, prior)
	if development.CaseCount == 0 {
		for i := range current.Floors {
			current.Floors[i].Status = "not_run"
		}
		for i := range current.Targets {
			current.Targets[i].Status = "not_run"
		}
		for i := range current.NonRegression {
			current.NonRegression[i].Status = "not_run"
		}
	} else {
		r.QualityPass = r.QualityPass && current.QualityPass
	}
	development.Floors, development.Targets, development.NonRegression = current.Floors, current.Targets, current.NonRegression
	r.Cohorts["development"] = development
}

func finishProductQuality(r *Report, previous *Report) {
	for _, mode := range []retrieval.Mode{"exact", "ann"} {
		for _, spec := range []struct {
			name          string
			limit         int
			threshold     float64
			gate, compare bool
		}{
			{"current_law_precision", 8, .99, true, false},
			{"recall", 20, .90, false, true}, {"mrr", 8, .66, false, true}, {"recall", 8, .90, false, false}, {"abstention_balanced_accuracy", 8, .85, false, true},
		} {
			m := findQualityMetric(r, mode, spec.name, spec.limit)
			if spec.gate {
				status := "unavailable"
				if m.Value != nil {
					status = "fail"
					if *m.Value >= spec.threshold {
						status = "pass"
					}
				}
				r.Floors = append(r.Floors, Floor{Metric: m, Floor: spec.threshold, Status: status})
				continue
			}
			status := "unavailable"
			if m.Value != nil {
				status = "below"
				if *m.Value >= spec.threshold {
					status = "met"
				}
			}
			r.Targets = append(r.Targets, Target{Metric: m, Target: spec.threshold, Status: status})
			if spec.compare {
				var old *float64
				if previous != nil {
					old = findQualityMetric(previous, mode, spec.name, spec.limit).Value
				}
				status = "no_previous"
				if m.Value == nil {
					status = "unavailable"
				} else if old != nil {
					status = "pass"
					if *m.Value < *old {
						status = "fail"
					}
				}
				r.NonRegression = append(r.NonRegression, NonRegression{Mode: mode, Arms: "hybrid", Limit: spec.limit, Name: spec.name, Value: m.Value, PreviousValue: old, Status: status, ValidityOverrides: r.ValidityOverrides, CapOverride: r.CapOverride, Reason: m.Reason, MappingIncompleteCount: m.MappingIncompleteCount})
			}
		}
	}
	r.QualityPass = len(r.Floors) == 2 && len(r.Rejections) == 3
	for _, gate := range r.Floors {
		r.QualityPass = r.QualityPass && gate.Status == "pass"
	}
	for _, rejection := range r.Rejections {
		r.QualityPass = r.QualityPass && rejection.Passed
	}
	for _, comparison := range r.NonRegression {
		if comparison.Name != "abstention_balanced_accuracy" {
			r.QualityPass = r.QualityPass && comparison.Status != "fail" && comparison.Status != "unavailable"
		}
	}
}

func findQualityMetric(r *Report, mode retrieval.Mode, name string, limit int) Metric {
	for _, m := range r.Cohorts["old_product"].Metrics {
		if m.Mode == mode && m.Arms == "hybrid" && m.Limit == limit && m.Name == name {
			return m
		}
	}
	return Metric{Mode: mode, Arms: "hybrid", Limit: limit, Name: name, ValidityOverrides: r.ValidityOverrides, CapOverride: r.CapOverride}
}
