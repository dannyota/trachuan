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
	var finalFailures []InvariantFailure
	err = copies.restore(ctx, req.DatabaseRequest, mutationSuffix(index), true, func(ctx context.Context, conn *pgx.Conn) error {
		current, closeCurrent := base, func() {}
		defer func() { closeCurrent() }()
		for _, class := range privateClasses() {
			positiveBefore, beforeErr := observePositive(ctx, current, req, class)
			closeCurrent()
			closeCurrent = func() {}
			if mutationErr := copies.mutate(ctx, conn, caller, class); mutationErr != nil {
				return mutationErr
			}
			var openErr error
			current, closeCurrent, openErr = copies.open(ctx, conn, key)
			if openErr != nil {
				closeCurrent = func() {}
				return openErr
			}
			positiveAfter, afterErr := observePositive(ctx, current, req, class)
			positive := positiveResult(name, class, positiveBefore, positiveAfter, beforeErr, afterErr)
			r.Classes[class] = positive.Classes[class]
			r.Pass = r.Pass && positive.Pass
			r.Failures = append(r.Failures, positive.Failures...)
		}
		after, observeErr := observe(ctx, current, req, caller)
		if observeErr != nil {
			finalFailures = failedInvariant(name, "observe_mutated", observeErr).Failures
		} else {
			finalFailures = comparisonFailures(before, after)
		}
		recalibrate(ctx, current, req, &r)
		return nil
	})
	if err != nil {
		return failedInvariant(name, "restore_mutate_or_cleanup", err)
	}
	if len(finalFailures) > 0 {
		r.Pass = false
		attributed, attributionErr := attributeFailures(ctx, req, copies, key, caller, name, index, before)
		if attributionErr != nil || len(attributed) == 0 {
			for i := range finalFailures {
				finalFailures[i].Class = "all"
			}
			attributed = append(attributed, finalFailures...)
		}
		r.Failures = append(r.Failures, attributed...)
		for _, failure := range attributed {
			if c, ok := r.Classes[failure.Class]; ok {
				c.Pass = false
				r.Classes[failure.Class] = c
			}
		}
	}
	return withFailureCauses(r)
}

func attributeFailures(ctx context.Context, req EvaluationRequest, copies invariantCopies, key []byte, caller serve.Caller, name string, index int, before []invariantObservation) ([]InvariantFailure, error) {
	failures := []InvariantFailure{}
	err := copies.restore(ctx, req.DatabaseRequest, mutationSuffix(index+100), true, func(ctx context.Context, conn *pgx.Conn) error {
		for _, class := range privateClasses() {
			if mutationErr := copies.mutate(ctx, conn, caller, class); mutationErr != nil {
				return mutationErr
			}
			engine, closeEngine, err := copies.open(ctx, conn, key)
			if err != nil {
				return err
			}
			after, observeErr := observe(ctx, engine, req, caller)
			closeEngine()
			stage := comparisonFailures(before, after)
			if observeErr != nil {
				stage = failedInvariant(name, "observe_mutated", observeErr).Failures
			}
			for _, failure := range stage {
				failure.Class = class
				failures = append(failures, failure)
			}
			if observeErr != nil {
				return nil
			}
			before = after
		}
		return nil
	})
	if err != nil {
		failures = append(failures, failedInvariant(name, "restore_attribution", err).Failures...)
	}
	return failures, err
}
