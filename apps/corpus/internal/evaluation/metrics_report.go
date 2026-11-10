package evaluation

import (
	"danny.vn/trachuan/internal/retrieval"
	"sort"
)

func finishReport(r *Report, all map[metricKey]*Metrics, modes []retrieval.Mode) {
	for _, cohort := range []string{"legacy", "old_product"} {
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
						metric := Metric{mode, arms, limit, name, num, den, value, conf}
						stats.Metrics = append(stats.Metrics, metric)
						if cohort == "old_product" && arms == "hybrid" && limit == 8 {
							floors := map[string]float64{"recall": .90, "mrr": .66, "current_law_precision": .99, "abstention": .95}
							if floor, ok := floors[name]; ok {
								status := "unavailable"
								if value != nil {
									status = "fail"
									if *value >= floor {
										status = "pass"
									}
								}
								r.Floors = append(r.Floors, Floor{metric, floor, status})
							}
						}
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
					sizes := m.EvidenceCharacters()
					sort.Float64s(sizes)
					r.EvidenceCharacters = append(r.EvidenceCharacters, SizeSummary{cohort, mode, arms, limit, len(sizes), Quantile(sizes, .5), Quantile(sizes, .95)})
				}
			}
		}
		r.Cohorts[cohort] = stats
	}
	addUnavailableFloors(r)
	r.QualityPass = len(r.Floors) == 8 && len(r.Rejections) == 3
	for _, f := range r.Floors {
		r.QualityPass = r.QualityPass && f.Status == "pass"
	}
	for _, v := range r.Rejections {
		r.QualityPass = r.QualityPass && v.Passed
	}
}

func markMappingUnavailable(r *Report) {
	stats := r.Cohorts["old_product"]
	for i := range stats.Metrics {
		m := &stats.Metrics[i]
		if m.Name == "recall" || m.Name == "macro_recall" || m.Name == "mrr" {
			m.Value = nil
		}
	}
	r.Cohorts["old_product"] = stats
	for i := range r.Floors {
		f := &r.Floors[i]
		if f.Name == "recall" || f.Name == "mrr" {
			f.Value = nil
			f.Status = "unavailable"
		}
	}
	r.QualityPass = false
}
