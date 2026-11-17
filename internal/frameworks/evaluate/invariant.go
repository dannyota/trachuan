package evaluate

import (
	"context"
	"errors"
	"slices"
	"strings"

	"danny.vn/trachuan/internal/frameworks/build"
	"danny.vn/trachuan/internal/frameworks/serve"
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
	r := InvariantCaller{Caller: name, Classes: map[string]InvariantClass{}, Calibration: "unavailable", Failures: []InvariantFailure{{Operation: op, Class: class, CaseID: id, Cause: build.IdentifyFailure(err)}}}
	for _, c := range privateClasses() {
		r.Classes[c] = InvariantClass{Status: "unavailable"}
	}
	return r
}

func checkInvariant(ctx context.Context, req EvaluationRequest, base EvaluationEngine, copies invariantCopies, key []byte, status serve.StatusResponse) InvariantResult {
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
		c := runCallerInvariant(ctx, req, base, copies, key, caller, name, i)
		r.Callers = append(r.Callers, c)
		r.Pass = r.Pass && c.Pass
	}
	return r
}
