package evaluate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"danny.vn/trachuan/internal/frameworks/build"
	"danny.vn/trachuan/internal/frameworks/serve"
	"github.com/jackc/pgx/v5"
)

func privateClasses() []string {
	return []string{"control_text", "body_index", "mapping_evidence", "text_gaps", "source_files"}
}
func operationClass(op string) string {
	switch op {
	case "read":
		return "control_text"
	case "search":
		return "body_index"
	case "mappings":
		return "mapping_evidence"
	case "quality_gaps":
		return "text_gaps"
	default:
		return "source_files"
	}
}
func compareInvariant(caller string, before, after, fullBefore, fullAfter []invariantObservation) InvariantCaller {
	r := InvariantCaller{Caller: caller, Classes: map[string]InvariantClass{}, Calibration: "unavailable", Failures: []InvariantFailure{}, Pass: true}
	positives := map[string]bool{}
	successfulBefore, successfulAfter := successfulObservations(fullBefore, fullAfter)
	original, mutated := privateObservations(successfulBefore), privateObservations(successfulAfter)
	for _, class := range privateClasses() {
		positives[class] = !bytes.Equal(original[class], mutated[class])
	}
	for _, class := range privateClasses() {
		r.Classes[class] = InvariantClass{Status: "evaluated", PositiveControl: positives[class], Pass: positives[class]}
		if !positives[class] {
			r.Pass = false
			r.Failures = append(r.Failures, InvariantFailure{Operation: "positive_control", Class: class})
		}
	}
	for _, observations := range [][]invariantObservation{before, after, fullBefore, fullAfter} {
		for _, o := range observations {
			if o.Error && !o.Diagnostic {
				r.Pass = false
				r.Failures = append(r.Failures, InvariantFailure{Operation: o.Operation, Class: operationClass(o.Operation), CaseID: o.CaseID, Mode: o.Mode, Page: o.Page, Repeat: o.Repeat})
			}
		}
	}
	before, after = successfulObservations(before, after)
	if len(before) != len(after) {
		r.Pass = false
		r.Failures = append(r.Failures, InvariantFailure{Operation: "response_count", Class: "all"})
	}
	for i, a := range before {
		if i >= len(after) {
			break
		}
		b := after[i]
		if a.Operation != b.Operation || a.CaseID != b.CaseID || a.Mode != b.Mode || a.Page != b.Page || a.Repeat != b.Repeat || !bytes.Equal(a.Value, b.Value) {
			class := operationClass(a.Operation)
			c := r.Classes[class]
			c.Pass = false
			r.Classes[class] = c
			r.Pass = false
			r.Failures = append(r.Failures, InvariantFailure{Operation: a.Operation, Class: class, CaseID: a.CaseID, Mode: a.Mode, Page: a.Page, Repeat: a.Repeat})
		}
	}
	return r
}
func failedInvariant(name, op string, err error) InvariantCaller {
	id := ""
	class := "all"
	var failure *caseFailure
	if errors.As(err, &failure) {
		id = failure.id
		op = failure.operation
		for _, operation := range []string{"search", "read", "mappings", "corpus_status", "quality_gaps"} {
			if strings.Contains(op, operation) {
				class = operationClass(operation)
				break
			}
		}
	}
	var mutation *mutationFailure
	if errors.As(err, &mutation) {
		class = mutation.class
		op = "mutation"
	}
	r := InvariantCaller{Caller: name, Classes: map[string]InvariantClass{}, Calibration: "unavailable", Failures: []InvariantFailure{{Operation: op, Class: class, CaseID: id}}}
	for _, c := range privateClasses() {
		r.Classes[c] = InvariantClass{Status: "unavailable"}
	}
	return r
}

func checkInvariant(ctx context.Context, req EvaluationRequest, base EvaluationEngine, restore Restore, open Opener, key []byte, status serve.StatusResponse) InvariantResult {
	codes := []string{}
	for h, v := range status.Versions {
		if v.Access == "licensed" {
			code := versionCode(h)
			if !slices.Contains(codes, code) {
				codes = append(codes, code)
			}
		}
	}
	slices.Sort(codes)
	r := InvariantResult{Pass: true, MixedCoverage: "not_applicable", Callers: []InvariantCaller{}}
	callers := []string{"free-only"}
	if len(codes) >= 2 {
		r.MixedCoverage = "evaluated"
		callers = append(callers, codes...)
	}
	for i, name := range callers {
		caller := freeCaller()
		if name != "free-only" {
			caller = serve.Caller{Readable: func(code, access string) bool { return access == "free" || code == name }}
		}
		if len(codes) == 0 {
			c := InvariantCaller{Caller: name, Classes: map[string]InvariantClass{}, Calibration: "not_applicable", Failures: []InvariantFailure{}, Pass: true}
			for _, class := range privateClasses() {
				c.Classes[class] = InvariantClass{Status: "not_applicable", Pass: true}
			}
			r.Callers = append(r.Callers, c)
			continue
		}
		c := runCallerInvariant(ctx, req, base, restore, open, key, caller, name, i)
		r.Callers = append(r.Callers, c)
		r.Pass = r.Pass && c.Pass
	}
	return r
}
func runCallerInvariant(ctx context.Context, req EvaluationRequest, base EvaluationEngine, restore Restore, open Opener, key []byte, caller serve.Caller, name string, index int) InvariantCaller {
	before, err := observe(ctx, base, req, caller)
	if err != nil {
		return failedInvariant(name, "observe_original", err)
	}
	fullBefore, err := observe(ctx, base, req, fullCaller())
	if err != nil {
		return failedInvariant(name, "positive_original", err)
	}
	r := failedInvariant(name, "restore_mutated", nil)
	err = restore(ctx, req.DatabaseRequest, mutationSuffix(index), true, func(ctx context.Context, conn *pgx.Conn) error {
		r = InvariantCaller{Caller: name, Classes: map[string]InvariantClass{}, Calibration: "unavailable", Failures: []InvariantFailure{}, Pass: true}
		for _, class := range privateClasses() {
			if mutationErr := mutateClass(ctx, conn, caller, class); mutationErr != nil {
				return mutationErr
			}
			after, fullAfter, stage, stageErr := observeMutationStage(ctx, req, conn, open, key, caller, name, class, before, fullBefore)
			if stageErr != nil {
				r = failedInvariant(name, "observe_mutated", stageErr)
				r.Failures[0].Class = class
				return nil
			}
			r.Classes[class] = stage.Classes[class]
			r.Pass = r.Pass && stage.Pass
			r.Failures = append(r.Failures, stage.Failures...)
			before, fullBefore = after, fullAfter
		}
		engine, closeEngine, openErr := open(ctx, conn, key)
		if openErr != nil {
			return openErr
		}
		defer closeEngine()
		calibration, calibrationErr := build.CalibrateCases(ctx, engine, req.Cases, req.CaseFileSHA256)
		if calibrationErr != nil || req.Calibration == nil || calibration.Threshold != req.Calibration.Threshold {
			r.Pass = false
			r.Calibration = "fail"
			r.Failures = append(r.Failures, InvariantFailure{Operation: "recalibration", Class: "all"})
		} else {
			r.Calibration = "pass"
		}

		return nil
	})
	if err != nil {
		return failedInvariant(name, "restore_mutate_or_cleanup", err)
	}
	return r
}

func privateObservations(observations []invariantObservation) map[string][]byte {
	values := map[string][]json.RawMessage{}
	for _, o := range observations {
		class := operationClass(o.Operation)
		var raw []byte
		switch o.Operation {
		case "read":
			raw = o.Text
		case "search":
			raw = o.Scores
		case "mappings":
			raw = o.Evidence
		default:
			raw = o.Value
		}
		if o.Operation == "read" || o.Operation == "search" || o.Operation == "mappings" {
			var parts []json.RawMessage
			if json.Unmarshal(raw, &parts) == nil {
				values[class] = append(values[class], parts...)
			}
		} else {
			values[class] = append(values[class], json.RawMessage(raw))
		}
	}
	result := map[string][]byte{}
	for _, class := range privateClasses() {
		result[class], _ = json.Marshal(values[class])
	}
	return result
}
