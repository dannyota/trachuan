package evaluate

import (
	"bytes"
	"context"
)

func positiveRequest(req EvaluationRequest, class string) EvaluationRequest {
	cases := req.Cases
	req.Cases = nil
	if class != "body_index" {
		return req
	}
	seen := map[string]bool{}
	for _, c := range cases {
		if c.Adjudication != "accepted" {
			continue
		}
		key := c.Code + ":" + c.Version
		if c.Negative {
			key = "negative"
		}
		if !seen[key] {
			req.Cases = append(req.Cases, c)
			seen[key] = true
		}
	}
	return req
}
func observePositive(ctx context.Context, e Engine, req EvaluationRequest, class string) ([]invariantObservation, error) {
	return observeOperations(ctx, e, positiveRequest(req, class), fullCaller(), class)
}
func positiveResult(name, class string, before, after []invariantObservation, beforeErr, afterErr error) InvariantCaller {
	if beforeErr != nil || afterErr != nil {
		if beforeErr == nil {
			beforeErr = afterErr
		}
		r := failedInvariant(name, "positive_control", beforeErr)
		r.Failures[0].Class = class
		return r
	}
	failures := observationFailures(before, after)
	if len(failures) > 0 {
		for i := range failures {
			failures[i].Class = class
		}
		return withFailureCauses(InvariantCaller{Caller: name, Classes: map[string]InvariantClass{class: {Status: "unavailable"}}, Failures: failures})
	}
	a, b := successfulObservations(before, after)
	positive := !bytes.Equal(privateObservations(a)[class], privateObservations(b)[class])
	r := InvariantCaller{Caller: name, Classes: map[string]InvariantClass{class: {Status: "evaluated", PositiveControl: positive, Pass: positive}}, Pass: positive, Failures: []InvariantFailure{}}
	if !positive {
		r.Failures = append(r.Failures, InvariantFailure{Operation: "positive_control", Class: class})
	}
	return withFailureCauses(r)
}
