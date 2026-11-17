package evaluate

import (
	"context"

	"danny.vn/trachuan/internal/frameworks/serve"
	"github.com/jackc/pgx/v5"
)

func observeMutationStage(ctx context.Context, req EvaluationRequest, conn *pgx.Conn, open Opener, key []byte, caller serve.Caller, name, class string, before, fullBefore []invariantObservation) ([]invariantObservation, []invariantObservation, InvariantCaller, error) {
	engine, closeEngine, err := open(ctx, conn, key)
	if err != nil {
		return nil, nil, InvariantCaller{}, err
	}
	defer closeEngine()
	after, err := observe(ctx, engine, req, caller)
	if err != nil {
		return nil, nil, InvariantCaller{}, err
	}
	fullAfter, err := observe(ctx, engine, req, fullCaller())
	if err != nil {
		return nil, nil, InvariantCaller{}, err
	}
	return after, fullAfter, compareMutationClass(name, class, before, after, fullBefore, fullAfter), nil
}

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
	return r
}
