package evaluate

import (
	"context"

	"danny.vn/trachuan/internal/frameworks/serve"
	"github.com/jackc/pgx/v5"
)

func runCallerInvariant(ctx context.Context, req EvaluationRequest, base Engine, copies invariantCopies, key []byte, caller serve.Caller, name string, index int) InvariantCaller {
	before, err := observe(ctx, base, req, caller)
	if err != nil {
		return failedInvariant(name, "observe_original", err)
	}
	r := InvariantCaller{Caller: name, Classes: map[string]InvariantClass{}, Calibration: "unavailable", Failures: []InvariantFailure{}, Pass: true}
	for _, class := range privateClasses() {
		r.Classes[class] = InvariantClass{Status: "unavailable"}
	}
	recalibrate(ctx, base, req, &r, "all")
	err = copies.restore(ctx, req.DatabaseRequest, mutationSuffix(index), true, func(ctx context.Context, conn *pgx.Conn) error {
		current, closeCurrent := base, func() {}
		defer func() { closeCurrent() }()
		for _, class := range privateClasses() {
			positiveBefore, beforeErr := observePositive(ctx, current, req, class)
			closeCurrent()
			closeCurrent = func() {}
			if mutationErr := copies.mutate(ctx, conn, caller, class); mutationErr != nil {
				recordStageFailure(&r, name, "mutation", class, mutationErr)
				return nil
			}
			var openErr error
			current, closeCurrent, openErr = copies.open(ctx, conn, key)
			if openErr != nil {
				closeCurrent = func() {}
				recordStageFailure(&r, name, "open_mutated", class, openErr)
				return nil
			}
			positiveAfter, afterErr := observePositive(ctx, current, req, class)
			positive := positiveResult(name, class, positiveBefore, positiveAfter, beforeErr, afterErr)
			r.Classes[class] = positive.Classes[class]
			r.Pass = r.Pass && positive.Pass
			r.Failures = append(r.Failures, positive.Failures...)
			after, observeErr := observe(ctx, current, req, caller)
			failures := comparisonFailures(before, after)
			if observeErr != nil {
				failures = failedInvariant(name, "observe_mutated", observeErr).Failures
			}
			c := r.Classes[class]
			for _, failure := range failures {
				failure.Class = class
				r.Failures = append(r.Failures, failure)
				c.Pass = false
			}
			if !recalibrate(ctx, current, req, &r, class) {
				c.Pass = false
			}
			r.Classes[class] = c
			r.Pass = r.Pass && c.Pass
		}
		return nil
	})
	if err != nil {
		return failedInvariant(name, "restore_mutate_or_cleanup", err)
	}
	return withFailureCauses(r)
}

func recordStageFailure(r *InvariantCaller, name, operation, class string, err error) {
	failure := failedInvariant(name, operation, err).Failures[0]
	failure.Class = class
	r.Failures = append(r.Failures, failure)
	r.Pass = false
	if r.Calibration != "fail" {
		r.Calibration = "unavailable"
	}
}
