package evaluation

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	"danny.vn/trachuan/internal/retrieval"
)

type policyEngine struct {
	fakeEngine
	primaryCount int
}

func TestValidityOverrides_ZeroTailKeepsCache(t *testing.T) {
	date := retrieval.Date{Year: 2026, Month: time.November, Day: 9}
	engine := &policyEngine{fakeEngine: fakeEngine{date: date}, primaryCount: 8}
	opts := Options{EvaluationDate: date, Modes: []retrieval.Mode{"exact"}}
	base, baseline, err := Run(context.Background(), engine, recordedSuite(), opts)
	if err != nil {
		t.Fatal(err)
	}
	zero := 0
	opts.NonCurrentTail = &zero
	trial, cache, err := Run(context.Background(), engine, recordedSuite(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(baseline.Arms, cache.Arms) || base.CacheManifest["cache_key"] != trial.CacheManifest["cache_key"] {
		t.Fatal("zero tail changed cached arms")
	}
}

func (e *policyEngine) collect(p retrieval.Prepared, depths retrieval.Depths, notYet bool) retrieval.Arms {
	arms := retrieval.Arms{Validation: p.Validation, Depths: depths, Date: e.date}
	if p.Request.Query != "Câu hỏi tốt" {
		return arms
	}
	var current []retrieval.Candidate
	for i := 1; i <= e.primaryCount; i++ {
		current = append(current, lossCandidate(int64(i), fmt.Sprintf("current-%d", i), "one"))
	}
	other := lossCandidate(50, "doc", "one")
	other.Document.IsCurrent = false
	other.Document.StatusClass = "expired"
	if notYet {
		other.Document.StatusClass = "not_yet"
		current = append([]retrieval.Candidate{other}, current...)
	}
	arms.Primary = retrieval.ArmGroup{Dense: current, Lexical: current, Members: current}
	if p.Request.InForceOnly != nil && !*p.Request.InForceOnly {
		arms.IncludeNonCurrent = true
		if !notYet {
			arms.NonCurrent = retrieval.ArmGroup{Dense: []retrieval.Candidate{other}, Lexical: []retrieval.Candidate{other}, Members: []retrieval.Candidate{other}}
		}
	}
	return arms
}

func (e *policyEngine) Collect(_ context.Context, p retrieval.Prepared, d retrieval.Depths) (retrieval.Arms, error) {
	return e.collect(p, d, false), nil
}

func (e *policyEngine) CollectWithNotYetEligible(_ context.Context, p retrieval.Prepared, d retrieval.Depths) (retrieval.Arms, error) {
	return e.collect(p, d, true), nil
}

func TestValidityOverrides_ReportAndCacheIdentity(t *testing.T) {
	history, tail := "agent", 1
	tests := []struct {
		name        string
		policy      ValidityOverrides
		count, rank int
		tag         bool
	}{
		{"historical", ValidityOverrides{HistoricalPolicy: &history}, 7, 8, true},
		{"tail", ValidityOverrides{NonCurrentTail: &tail}, 8, 8, false},
		{"not_yet", ValidityOverrides{NotYetEligible: true}, 7, 1, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			date := retrieval.Date{Year: 2026, Month: time.November, Day: 9}
			engine := &policyEngine{fakeEngine: fakeEngine{date: date}, primaryCount: tc.count}
			suite := recordedSuite()
			if tc.tag {
				suite.Legacy[0].Tags = []string{"historical"}
			}
			opts := Options{EvaluationDate: date, Modes: []retrieval.Mode{"exact"}}
			base, baseCache, err := Run(context.Background(), engine, suite, opts)
			if err != nil {
				t.Fatal(err)
			}
			opts.ValidityOverrides = tc.policy
			report, cache, err := Run(context.Background(), engine, suite, opts)
			if err != nil {
				t.Fatal(err)
			}
			var before, after CaseResult
			for _, c := range base.Cases {
				if c.CaseID == "positive" && c.Cohort == "old_product" && c.Arms == "hybrid" && c.Limit == 8 {
					before = c
					break
				}
			}
			for _, c := range report.Cases {
				if c.CaseID == "positive" && c.Cohort == "old_product" && c.Arms == "hybrid" && c.Limit == 8 {
					after = c
					break
				}
			}
			if len(before.Ranks) != 1 || before.Ranks[0] != 0 || len(after.Ranks) != 1 || after.Ranks[0] != tc.rank || after.Matched != 1 || len(after.Returned) < tc.rank || after.Returned[tc.rank-1].DocumentKey != "doc" {
				t.Fatalf("baseline %+v trial %+v", before, after)
			}
			if base.CacheManifest["cache_key"] == report.CacheManifest["cache_key"] || baseCache.Arms[0].EligibilityHash == cache.Arms[0].EligibilityHash {
				t.Fatal("collection policy reused cache identity")
			}
			data, err := json.Marshal(report)
			if err != nil {
				t.Fatal(err)
			}
			var root any
			if err = json.Unmarshal(data, &root); err != nil {
				t.Fatal(err)
			}
			var visit func(any)
			visit = func(value any) {
				switch v := value.(type) {
				case []any:
					for _, child := range v {
						visit(child)
					}
				case map[string]any:
					if _, ok := v["limit"]; ok {
						var wantHistory any
						var wantTail any
						if tc.policy.HistoricalPolicy != nil {
							wantHistory = *tc.policy.HistoricalPolicy
						}
						if tc.policy.NonCurrentTail != nil {
							wantTail = float64(*tc.policy.NonCurrentTail)
						}
						if v["historical_policy"] != wantHistory || v["non_current_tail"] != wantTail || v["not_yet_eligible"] != tc.policy.NotYetEligible {
							t.Errorf("limit record missing policy: %v", v)
						}
					}
					for _, child := range v {
						visit(child)
					}
				}
			}
			visit(root)
		})
	}
}
