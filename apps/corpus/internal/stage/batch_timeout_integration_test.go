//go:build integration

package stage

import (
	"context"
	"errors"
	"testing"
)

func TestRunBatch_PreviousRetryableCause(t *testing.T) {
	for _, tc := range []struct {
		name                        string
		first                       Cause
		firstAttempt, secondAttempt int
		newGeneration               bool
		outcome                     string
	}{
		{"rate limit then timeout", CauseHTTP429, 1, 2, false, "pending"},
		{"timeout then timeout", CauseTimeout, 1, 2, false, "gap"},
		{"missed attempt", CauseTimeout, 1, 3, false, "pending"},
		{"new generation", CauseTimeout, 1, 2, true, "pending"},
		{"first timeout on final attempt", CauseHTTP429, 4, 5, false, "failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env, id, s := batchSetup(t)
			cause := tc.first
			s.process = func(context.Context, Env, Run, Item) (Output, error) {
				return Output{}, &RetryableError{Err: errors.New("source failure"), GapKind: "source_unavailable", Cause: GapCause{Source: "vbpl", Origin: OriginTransport, Cause: cause, Endpoint: "doc/diagram"}}
			}
			if _, err := batchRun(env, id, s, tc.firstAttempt, func(...any) {}); err == nil {
				t.Fatal("first attempt succeeded")
			}
			if tc.newGeneration {
				batchExec(t, env.DB, `UPDATE run SET generation=3 WHERE id=$1`, id)
			}
			cause = CauseTimeout
			in := BatchInput{RunID: id, Generation: 2, Stage: "fetch"}
			if tc.newGeneration {
				in.Generation = 3
			}
			result, err := RunBatch(context.Background(), env, s, in, tc.secondAttempt, BatchConfig{HeartbeatEvery: 1e9, MaxAttempts: 5}, func(...any) {})
			switch tc.outcome {
			case "gap":
				if err != nil || result.Gap != 1 {
					t.Fatalf("result=%+v err=%v", result, err)
				}
			case "failed":
				if err != nil || result.Failed != 1 {
					t.Fatalf("result=%+v err=%v", result, err)
				}
			case "pending":
				if err == nil || result.Pending != 1 {
					t.Fatalf("result=%+v err=%v", result, err)
				}
			}
		})
	}
}
