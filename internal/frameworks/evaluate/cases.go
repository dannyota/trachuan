package evaluate

import (
	"context"

	"danny.vn/trachuan/internal/frameworks/businesskey"
	"danny.vn/trachuan/internal/frameworks/serve"
)

func freeCaller() serve.Caller {
	return serve.Caller{Readable: func(_ string, access string) bool { return access == "free" }}
}
func fullCaller() serve.Caller {
	return serve.Caller{Readable: func(string, string) bool { return true }}
}
func caseRequest(c GoldenCase, mode string) serve.Request {
	r := serve.Request{Query: c.Question, Limit: 8}
	if mode == "filtered" {
		r.Filters = map[string][]string{"frameworks": {c.Code}}
	}
	return r
}
func controlHandle(c Control) string {
	return "fw:" + c.Code + ":" + c.Version + "#" + businesskey.EncodeCitation(c.Citation)
}
func caseOutcome(c GoldenCase, response serve.Response, err error, controls map[string]Control) CaseOutcome {
	o := CaseOutcome{Miss: !c.Negative, Abstention: "answered", NearHits: []NearHit{}}
	if err != nil {
		o.Error = true
		o.Abstention = "error"
		return o
	}
	if response.Abstain {
		o.Abstention = "abstained"
	}
	target := controls[c.Target]
	for i, h := range response.Hits[:min(8, len(response.Hits))] {
		if h.Handle == c.Target {
			o.Rank = i + 1
			o.Miss = false
		}
		hit, ok := controls[h.Handle]
		if !ok || c.Negative {
			continue
		}
		if hit.Code == c.Code {
			o.FrameworkHits++
			if hit.Version == c.Version {
				o.CurrentHits++
			}
		}
		kind := ""
		switch {
		case hit.Code == c.Code && hit.Version != c.Version:
			kind = "other_version"
		case hit.Code == c.Code && hit.Version == c.Version && target.Parent != nil && *target.Parent == hit.Citation:
			kind = "parent"
		case hit.Code == c.Code && hit.Version == c.Version && hit.Parent != nil && *hit.Parent == target.Citation:
			kind = "child"
		}
		if kind != "" {
			o.NearHits = append(o.NearHits, NearHit{Handle: h.Handle, Kind: kind, Rank: i + 1})
		}
	}
	return o
}

func scoreCases(ctx context.Context, e EvaluationEngine, request EvaluationRequest) []CaseResult {
	controls := map[string]Control{}
	for _, c := range request.Controls {
		controls[controlHandle(c)] = c
	}
	results := make([]CaseResult, 0, len(request.Cases))
	for _, c := range request.Cases {
		r := CaseResult{ID: c.ID, Adjudication: c.Adjudication, Target: c.Target, Code: c.Code, Version: c.Version, Access: c.Access, Negative: c.Negative, Results: map[string]map[string]CaseOutcome{}, Filter: map[string]string{}}
		if c.Adjudication == "accepted" || c.Adjudication == "unverified" {
			for _, name := range []string{"free-only", "full"} {
				caller := freeCaller()
				if name == "full" {
					caller = fullCaller()
				}
				r.Results[name] = map[string]CaseOutcome{}
				for _, mode := range []string{"open", "filtered"} {
					if c.Negative && mode == "filtered" {
						continue
					}
					response, err := e.Search(ctx, caseRequest(c, mode), caller)
					r.Results[name][mode] = caseOutcome(c, response, err, controls)
				}
			}
		}
		r.Cohorts = map[string]map[string]CaseOutcome{}
		for _, cohort := range []string{"public", "licensed"} {
			if len(r.Results) > 0 && (cohort == "licensed" || r.Access == "free" || r.Negative) {
				r.Cohorts[cohort] = r.Results[cohortCaller(cohort)]
			}
			r.Filter[cohort] = "not_applicable"
			if c.Negative || r.Adjudication != "accepted" || (cohort != "licensed" && r.Access != "free") {
				continue
			}
			a, b := r.Results[cohortCaller(cohort)]["open"], r.Results[cohortCaller(cohort)]["filtered"]
			r.Filter[cohort] = "pass"
			if a.Error || b.Error {
				r.Filter[cohort] = "unavailable"
			} else if a.Rank > 0 && b.Rank == 0 {
				r.Filter[cohort] = "fail"
			}
		}
		results = append(results, r)
	}
	return results
}
