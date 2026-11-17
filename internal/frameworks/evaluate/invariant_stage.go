package evaluate

import (
	"context"

	"danny.vn/trachuan/internal/frameworks/build"
)

func compareMutationClass(name, class string, before, after, fullBefore, fullAfter []invariantObservation) InvariantCaller {
	all := compareInvariant(name, before, after, fullBefore, fullAfter)
	positive := all.Classes[class].PositiveControl
	r := InvariantCaller{Caller: name, Classes: map[string]InvariantClass{}, Pass: positive, Failures: []InvariantFailure{}}
	if !positive {
		r.Failures = append(r.Failures, InvariantFailure{Operation: "positive_control", Class: class})
	}
	for _, failure := range all.Failures {
		if failure.Operation == "positive_control" {
			continue
		}
		failure.Class = class
		r.Failures = append(r.Failures, failure)
		r.Pass = false
	}
	r.Classes[class] = InvariantClass{Status: "evaluated", PositiveControl: positive, Pass: r.Pass}
	return withFailureCauses(r)
}

func recalibrate(ctx context.Context, engine Engine, req EvaluationRequest, r *InvariantCaller, class string) bool {
	calibration, err := build.CalibrateCases(ctx, engine, req.Cases, req.CaseFileSHA256)
	if err != nil || req.Calibration == nil || calibration.Threshold != req.Calibration.Threshold {
		r.Pass = false
		r.Calibration = "fail"
		r.Failures = append(r.Failures, InvariantFailure{Operation: "recalibration", Class: class, Cause: build.IdentifyFailure(err)})
		return false
	}
	if r.Calibration != "fail" {
		r.Calibration = "pass"
	}
	return true
}
