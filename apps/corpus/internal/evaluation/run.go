package evaluation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"time"
	"unicode/utf8"

	"danny.vn/trachuan/internal/retrieval"
)

type metricKey struct {
	cohort string
	mode   retrieval.Mode
	arms   retrieval.ArmMode
	limit  int
}

// Run scores legacy, old-product and development cases with a frozen recipe.
func Run(ctx context.Context, engine Engine, suite Suite, o Options) (Report, Cache, error) {
	if err := validateValidityOverrides(o.ValidityOverrides); err != nil {
		return Report{}, Cache{}, err
	}
	if _, err := json.Marshal(o.CapOverride); err != nil {
		return Report{}, Cache{}, fmt.Errorf("invalid cap override: %w", err)
	}
	if err := validateSuite(suite); err != nil {
		return Report{}, Cache{}, err
	}
	date := o.EvaluationDate
	dt := time.Date(date.Year, date.Month, date.Day, 0, 0, 0, 0, time.UTC)
	if date.Year < 1 || date.Year > 9999 || dt.Year() != date.Year || dt.Month() != date.Month || dt.Day() != date.Day {
		return Report{}, Cache{}, fmt.Errorf("invalid evaluation date")
	}
	modes := o.Modes
	if len(modes) == 0 {
		modes = []retrieval.Mode{"exact", "ann"}
	}
	seenModes := map[retrieval.Mode]bool{}
	for _, m := range modes {
		if (m != "exact" && m != "ann") || seenModes[m] {
			return Report{}, Cache{}, fmt.Errorf("invalid evaluation modes")
		}
		seenModes[m] = true
	}
	rel := engine.Release()
	o = withReleaseRecipes(o, rel, modes)
	recipe := rel.RetrievalRecipe
	r := Report{Version: 1, Scorer: "agent-eval/2", Status: "complete", Country: rel.Country, Rel: rel.Rel, BuildArtifactSHA256: o.BuildArtifactSHA256, DumpSHA256: o.DumpSHA256, FixtureHashes: suite.Hashes, IdentityMapSHA256: suite.Hashes.IdentityMap, Recipes: o.Recipes, EvaluationDate: date, DatabaseInstanceKind: o.DatabaseInstanceKind, Software: o.Software, Calibration: o.Calibration, Cohorts: map[string]Cohort{"legacy": {}, "old_product": {}}, TimingsMS: map[string]any{}, CacheManifest: map[string]any{}, PreviousDiff: map[string]any{"reason": "no previous report"}, NotRun: map[string]string{"development": "not_run", "taxonomy": "out_of_scope", "filtered": "out_of_scope"}, Limitations: []string{"Small sample size; calibration fits the scored product cohorts.", "Legacy results use the product recipe and are not a reproduction of the legacy engine."}}
	r.CapOverride = o.CapOverride
	r.ValidityOverrides = o.ValidityOverrides
	var previous *Report
	if o.PreviousReport != "" {
		b, err := os.ReadFile(o.PreviousReport)
		if err != nil {
			return Report{}, Cache{}, fmt.Errorf("read previous report: %w", err)
		}
		var prior Report
		if err = json.Unmarshal(b, &prior); err != nil {
			return Report{}, Cache{}, fmt.Errorf("decode previous report: %w", err)
		}
		if prior.Version != 1 || (prior.Scorer != r.Scorer && prior.Scorer != "agent-eval/1") || prior.FixtureHashes.Legacy != r.FixtureHashes.Legacy || prior.FixtureHashes.OldProduct != r.FixtureHashes.OldProduct {
			return Report{}, Cache{}, fmt.Errorf("incompatible previous scorer or fixtures")
		}
		if prior.Cohorts["development"].CaseCount > 0 && prior.FixtureHashes.Development != r.FixtureHashes.Development {
			return Report{}, Cache{}, fmt.Errorf("incompatible previous development fixtures")
		}
		if prior.Scorer != r.Scorer {
			r.PreviousDiff = map[string]any{"reason": "incompatible abstention policy scorer"}
		} else if (prior.CapOverride == nil) != (o.CapOverride == nil) || (prior.CapOverride != nil && o.CapOverride != nil && *prior.CapOverride != *o.CapOverride) {
			r.PreviousDiff = map[string]any{"reason": "incompatible cap override"}
		} else if !sameValidityOverrides(prior.ValidityOverrides, o.ValidityOverrides) {
			r.PreviousDiff = map[string]any{"reason": "incompatible validity policy"}
		} else {
			previous = &prior
		}
	}
	rejects := rejectionSet(suite)
	entries := map[string]map[int]MapEntry{}
	for _, e := range suite.Map.Entries {
		if entries[e.CaseID] == nil {
			entries[e.CaseID] = map[int]MapEntry{}
		}
		entries[e.CaseID][e.CitationIndex] = e
		if e.State == "absent" {
			for _, c := range append(append([]Case{}, suite.Legacy...), suite.Development...) {
				if c.ID == e.CaseID && e.CitationIndex >= 0 && e.CitationIndex < len(c.ExpectedCitations) {
					x := c.ExpectedCitations[e.CitationIndex]
					r.Coverage = append(r.Coverage, Coverage{c.ID, e.CitationIndex, append([]string{x.DocNumber}, x.AltDocNumbers...)})
				}
			}
		}
	}
	filter, err := newFilterState(ctx, engine, suite, o)
	if err != nil {
		return Report{}, Cache{}, err
	}
	if filter.active {
		delete(r.NotRun, "filtered")
	} else if filter.gate.Reason != "" {
		r.NotRun["filtered"] = filter.gate.Reason
	}
	inputs := cohortInputs(suite, &r)
	cache := Cache{}
	metrics := map[metricKey]*Metrics{}
	presented := map[presentedKey]*presentedSamples{}
	timings := map[string][]float64{}
	vectorRows := map[string]int{}
	for _, input := range inputs {
		c := input.c
		force := inForceForCase(c, o.ValidityOverrides)
		request := retrieval.Request{Query: c.Question, Limit: 8, Mode: "exact", Arms: "hybrid", InForceOnly: &force}
		product, err := engine.Prepare(ctx, request)
		if err != nil {
			return Report{}, Cache{}, fmt.Errorf("prepare case %s: %w", c.ID, err)
		}
		legacy := product
		if input.legacy && !product.Validation.Accepted {
			legacy, err = engine.PrepareLegacy(ctx, request)
			if err != nil {
				return Report{}, Cache{}, fmt.Errorf("prepare legacy case %s: %w", c.ID, err)
			}
		}
		if input.legacy && (!legacy.Validation.Accepted || legacy.Vector == nil) {
			return Report{}, Cache{}, fmt.Errorf("legacy case %s rejected", c.ID)
		}
		if input.legacy && rejects[c.ID] {
			v := product.Validation
			r.Rejections = append(r.Rejections, Rejection{c.ID, v.Reason, !v.Accepted && product.Vector == nil && v.Reason == "diacritics_required" && v.Message == "Vui lòng nhập câu hỏi tiếng Việt có dấu hoặc số hiệu văn bản."})
		} else if !product.Validation.Accepted || product.Vector == nil {
			return Report{}, Cache{}, fmt.Errorf("product case %s unexpectedly rejected", c.ID)
		}
		for _, cohort := range input.cohorts {
			stats := r.Cohorts[cohort]
			stats.CaseCount++
			if cohort == "old_product" && rejects[c.ID] {
				stats.RejectionCount++
				cache.Index = append(cache.Index, QueryIndex{CaseID: c.ID, Cohort: cohort, Reason: product.Validation.Reason})
				r.Cohorts[cohort] = stats
				continue
			}
			if c.ExpectAbstain {
				stats.NegativeCount++
			} else {
				stats.PositiveCount++
				stats.ExpectedCitations += len(c.ExpectedCitations)
			}
			r.Cohorts[cohort] = stats
			p := product
			if cohort == "legacy" {
				p = legacy
			}
			hash := sha256.Sum256([]byte(c.Question))
			queryHash := hex.EncodeToString(hash[:])
			key := queryHash + ":" + p.Vector.RecipeHash
			row, ok := vectorRows[key]
			if !ok {
				row = len(cache.Vectors)
				vectorRows[key] = row
				cache.Vectors = append(cache.Vectors, append([]float32(nil), p.Vector.Values...))
			}
			rowCopy := row
			cache.Index = append(cache.Index, QueryIndex{CaseID: c.ID, Cohort: cohort, Row: &rowCopy, Count: len(p.Vector.Values), QuerySHA256: queryHash, RecipeHash: p.Vector.RecipeHash})
			collected := map[retrieval.Mode]retrieval.Arms{}
			for _, mode := range modes {
				p.Request.Mode = mode
				depths := retrieval.Depths{Dense: recipe.DenseDepth, Lexical: recipe.LexicalDepth}
				if mode == "exact" {
					depths = retrieval.Depths{Dense: 200, Lexical: 100}
				}
				started := time.Now()
				arms, err := collectEvaluation(ctx, engine, p, depths, force, o.ValidityOverrides)
				timingKey := cohort + "/" + string(mode)
				timings[timingKey] = append(timings[timingKey], float64(time.Since(started))/float64(time.Millisecond))
				if err != nil {
					return Report{}, Cache{}, fmt.Errorf("collect case %s: %w", c.ID, err)
				}
				if arms.Date != date || arms.AsOf != nil {
					return Report{}, Cache{}, fmt.Errorf("evaluation date mismatch for case %s", c.ID)
				}
				if cohort == "old_product" && len(o.Mapping[c.ID]) == 0 {
					if err = filter.addCase(c, entries[c.ID], arms, recipe, p.Request); err != nil {
						return Report{}, Cache{}, err
					}
				}
				collected[mode] = arms
				eligibility := validityEligibilityHash(date, force, o.ValidityOverrides, arms)
				ar := ArmRow{Version: 1, CaseID: c.ID, Cohort: cohort, ReleaseHash: o.DumpSHA256, RecipeHashes: o.Recipes, EvaluationDate: date, Mode: mode, Depths: arms.Depths, Route: arms.Validation.Routed, EligibilityHash: eligibility, Dense: cachedCandidates(arms.Primary.Dense, false), Lexical: cachedCandidates(arms.Primary.Lexical, true), RawANN: cachedCandidates(arms.Primary.RawANN, false), Fallback: arms.Primary.Fallback, Diagnostics: map[string]any{}}
				for _, armMode := range []retrieval.ArmMode{"hybrid", "dense", "lexical"} {
					for _, limit := range []int{8, 20} {
						var response retrieval.Response
						if force && o.NonCurrentTail != nil {
							response, err = retrieval.ShapeWithNonCurrentTail(arms, recipe, limit, armMode, o.CapOverride, *o.NonCurrentTail)
						} else {
							response, err = retrieval.ShapeWithCap(arms, recipe, limit, armMode, o.CapOverride)
						}
						if err != nil {
							return Report{}, Cache{}, fmt.Errorf("shape case %s: %w", c.ID, err)
						}
						ranking, err := retrieval.Rank(arms, recipe, limit, armMode)
						if err != nil {
							return Report{}, Cache{}, fmt.Errorf("rank case %s: %w", c.ID, err)
						}
						for _, g := range ranking.Groups {
							ar.Groups = append(ar.Groups, CachedGroup{armMode, limit, g.Identity, g.Score, g.BestCosine})
						}
						if limit == 8 {
							r.addHits(c.ID, response.Hits, suite.ValidityCases)
						}
						hits := append(append([]retrieval.Hit{}, response.Hits...), response.NonCurrent...)
						ranks := make([]int, len(c.ExpectedCitations))
						direct := make([]int, len(ranks))
						for i, e := range c.ExpectedCitations {
							if cohort == "legacy" {
								ranks[i], _ = MatchLegacy(e, hits)
								e.RelationOK = false
								direct[i], _ = MatchLegacy(e, hits)
							} else {
								if len(o.Mapping[c.ID]) == 0 {
									ranks[i] = MatchProduct(entries[c.ID][i], hits)
								}
							}
						}
						if armMode == "hybrid" {
							for _, detail := range []retrieval.Detail{retrieval.DetailCompact, retrieval.DetailFull} {
								result, presentErr := engine.Present(ctx, response, detail)
								if presentErr != nil {
									return Report{}, Cache{}, fmt.Errorf("present case %s: %w", c.ID, presentErr)
								}
								data, marshalErr := json.Marshal(result)
								if marshalErr != nil {
									return Report{}, Cache{}, fmt.Errorf("encode presentation for case %s: %w", c.ID, marshalErr)
								}
								key := presentedKey{cohort, mode, detail, limit}
								if presented[key] == nil {
									presented[key] = &presentedSamples{}
								}
								sample := presented[key]
								sample.cases++
								sample.expected += len(ranks)
								sample.sizes = append(sample.sizes, float64(utf8.RuneCount(data)))
								if detail == retrieval.DetailFull {
									sample.found += matchedCount(ranks)
									continue
								}
								compact := compactHits(hits)
								for i, e := range c.ExpectedCitations {
									if cohort == "legacy" {
										rank, _ := MatchLegacy(e, compact)
										if rank > 0 {
											sample.found++
										}
									} else if len(o.Mapping[c.ID]) == 0 && MatchProduct(entries[c.ID][i], compact) > 0 {
										sample.found++
									}
								}
							}
						}
						key := metricKey{cohort, mode, armMode, limit}
						if metrics[key] == nil {
							metrics[key] = &Metrics{}
						}
						score := CaseScore{Negative: c.ExpectAbstain, Ranks: ranks, Response: response}
						metrics[key].Cases = append(metrics[key].Cases, score)
						chars := int((Metrics{Cases: []CaseScore{score}}).EvidenceCharacters()[0])
						diagnosticEntries := entries[c.ID]
						if cohort != "legacy" && len(o.Mapping[c.ID]) > 0 {
							diagnosticEntries = nil
						}
						diagnosticRecipe := recipe
						if o.CapOverride != nil {
							diagnosticRecipe.DocumentCap = int(*o.CapOverride)
							if diagnosticRecipe.DocumentCap == 0 {
								diagnosticRecipe.DocumentCap = limit
							}
						}
						var diagnosticTail *int
						if force {
							diagnosticTail = o.NonCurrentTail
						}
						losses, gaps, err := caseDiagnostics(arms, diagnosticRecipe, limit, armMode, cohort, c.ExpectedCitations, diagnosticEntries, response, diagnosticTail)
						if err != nil {
							return Report{}, Cache{}, fmt.Errorf("diagnose case %s: %w", c.ID, err)
						}
						result := CaseResult{ValidityOverrides: o.ValidityOverrides, CapOverride: o.CapOverride, CaseID: c.ID, Cohort: cohort, Mode: mode, Arms: armMode, Limit: limit, Expected: len(ranks), Ranks: ranks, Matched: matchedCount(ranks), Mapped: caseMappings(entries[c.ID], len(ranks)), Abstain: response.Abstain, Reason: response.Reason, Gaps: gaps, Losses: losses, Characters: chars}
						if cohort != "legacy" {
							result.MappingIssues = o.Mapping[c.ID]
						}
						if cohort == "legacy" {
							result.DirectRanks = direct
						}
						for _, hit := range hits {
							result.Returned = append(result.Returned, hit.Identity)
						}
						r.Cases = append(r.Cases, result)
					}
				}
				cache.Arms = append(cache.Arms, ar)
			}
			if exact, ok := collected["exact"]; ok {
				if ann, ok := collected["ann"]; ok {
					r.ANN = append(r.ANN, annMeasure(c.ID, cohort, exact.Primary.Dense, ann.Primary.RawANN, ann.Primary.Dense, ann.Primary.Fallback, recipe.DenseDepth))
				}
			}
		}
	}
	for key, samples := range timings {
		r.TimingsMS[key] = summarizeTiming(samples)
	}
	finishReport(&r, metrics, modes)
	finishPresented(&r, presented, modes)
	finishMappingReport(&r, inputs, rejects, o.Mapping)
	finishQuality(&r, previous)
	filter.finish(modes)
	r.FilterGate = filter.gate
	if filter.active {
		for _, gate := range r.FilterGate.Modes {
			r.QualityPass = r.QualityPass && gate.Status == "pass"
		}
	}
	addLegacyDirectMetrics(&r)
	r.ANNResults = map[string]ANNLoss{}
	for _, cohort := range reportCohorts(&r) {
		subset := r
		subset.ANN = nil
		for _, item := range r.ANN {
			if item.Cohort == cohort {
				subset.ANN = append(subset.ANN, item)
			}
		}
		r.ANNResults[cohort] = summarizeANN(subset, cohort)
	}
	if previous != nil {
		r.PreviousDiff = map[string]any{"label": "corpus changes are coverage changes", "cases": compareCases(previous.Cases, r.Cases)}
	}
	r.CacheManifest = map[string]any{"arm_rows": len(cache.Arms), "query_rows": len(cache.Vectors), "query_index_entries": len(cache.Index), "cache_key": cacheKey(cache, o, suite.Hashes)}
	return r, cache, nil
}
func annMeasure(id, cohort string, exact, raw, effective []retrieval.Candidate, fallback bool, depth int) ANNMeasure {
	exact = exact[:min(len(exact), depth)]
	return ANNMeasure{id, cohort, len(exact), overlap(exact, raw), overlap(exact, effective), fallback}
}
func overlap(exact, other []retrieval.Candidate) int {
	ids := map[int64]bool{}
	for _, c := range exact {
		ids[c.ChunkID] = true
	}
	n := 0
	for _, c := range other {
		if ids[c.ChunkID] {
			n++
		}
	}
	return n
}
