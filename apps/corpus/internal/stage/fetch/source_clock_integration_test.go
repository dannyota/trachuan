//go:build integration

package fetch

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/sourcegate"
	"danny.vn/trachuan/apps/corpus/internal/stage"
)

// Only source work uses virtual time; batch database work stays outside the bubble.
// These error fixtures do not produce bundles or file hashes to persist.
type clockedSourceStage struct {
	*Stage
	test    *testing.T
	prepare func(context.Context) error
}

func TestClockedSource_Failures(t *testing.T) {
	for _, name := range []string{"page 429", "PDF 429", "unexpected 206", "pending pause"} {
		t.Run(name, func(t *testing.T) {
			var processor clockedSourceStage
			key := "vn:chinhphu:218029"
			var calls *int
			var env stage.Env
			switch name {
			case "pending pause":
				processor, key = pausePersistenceFixture(t), "vn:vbpl:1"
			case "unexpected 206":
				s, environment, count := unexpected206Fixture(t, time.Millisecond)
				processor, calls = clockedSourceStage{Stage: s, test: t}, count
				env = environment
			default:
				s, environment, count, _ := stalled429Fixture(t, name == "PDF 429", time.Millisecond)
				processor, calls = clockedSourceStage{Stage: s, test: t}, count
				env = environment
			}
			out, err := processor.Process(context.Background(), env, stage.Run{ID: 1, Country: "vn"}, stage.Item{Key: key})
			switch name {
			case "unexpected 206":
				if err != nil || *calls != 1 || out.State != "gap" || len(out.Gaps) != 1 || out.Gaps[0].HTTPStatus == nil || *out.Gaps[0].HTTPStatus != 206 {
					t.Fatalf("output=%+v calls=%d error=%v", out, *calls, err)
				}
			case "pending pause":
				var pending interface{ PausePending() bool }
				if !errors.Is(err, sourcegate.ErrSourcePaused) || !errors.As(err, &pending) || !pending.PausePending() {
					t.Fatalf("pending pause=%v", err)
				}
			default:
				var retry *stage.RetryableError
				if *calls != 4 || !errors.As(err, &retry) || retry.Cause.Cause != stage.CauseHTTP429 {
					t.Fatalf("calls=%d error=%v", *calls, err)
				}
			}
		})
	}
}

func (s clockedSourceStage) Process(ctx context.Context, env stage.Env, run stage.Run, item stage.Item) (out stage.Output, err error) {
	if err = ctx.Err(); err != nil {
		return out, err
	}
	synctest.Test(s.test, func(t *testing.T) {
		activity, cancel := context.WithCancel(context.Background())
		defer cancel()
		activity = sourcegate.WithActivity(sourcegate.WithItem(sourcegate.WithRunID(activity, run.ID), item.Key))
		if s.prepare != nil {
			if e := s.prepare(activity); e != nil {
				t.Fatal(e)
			}
		}
		env.DB = nil
		out, err = s.Stage.Process(activity, env, run, item)
	})
	return out, err
}
