//go:build integration

package stage

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestRunBatch_SourceRule(t *testing.T) {
	for _, cause := range []Cause{CauseTimeout, CauseConnectionError, CauseHTTP5xx, CauseNonJSON200, CauseUnknown, CauseHTTP429} {
		for _, attempt := range []int{1, 5} {
			t.Run(string(cause)+string(rune('0'+attempt)), func(t *testing.T) {
				env, id, s := batchSetup(t)
				s.process = func(context.Context, Env, Run, Item) (Output, error) {
					return Output{}, &RetryableError{Err: errors.New("source unavailable"), GapKind: "source_unavailable", Cause: GapCause{Cause: cause}}
				}
				result, err := batchRun(env, id, s, attempt, func(...any) {})
				if cause == CauseHTTP429 {
					if err == nil || result.Pending != 1 {
						t.Fatalf("%+v %v", result, err)
					}
				} else if err != nil || result.Gap != 1 {
					t.Fatalf("%+v %v", result, err)
				}
				var message *string
				if e := env.DB.QueryRow(context.Background(), `SELECT message FROM run_item WHERE run_id=$1`, id).Scan(&message); e != nil {
					t.Fatal(e)
				}
				if message != nil && strings.HasPrefix(*message, "retry:") {
					t.Fatalf("kept previous cause: %s", *message)
				}
			})
		}
	}
}
