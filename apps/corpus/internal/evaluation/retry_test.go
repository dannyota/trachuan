package evaluation

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"danny.vn/trachuan/internal/retrieval"
	"github.com/jackc/pgx/v5/pgconn"
)

type retryEngine struct {
	cohortCalibrationEngine
	mu             sync.Mutex
	target         string
	failures       int
	cause          error
	calls          map[string]int
	active         int
	retryAlone     bool
	completed      map[string]bool
	allBeforeRetry bool
	completedWant  int
}

func (e *retryEngine) Collect(ctx context.Context, p retrieval.Prepared, d retrieval.Depths) (retrieval.Arms, error) {
	e.mu.Lock()
	e.active++
	e.calls[p.Request.Query]++
	attempt := e.calls[p.Request.Query]
	if p.Request.Query == e.target && attempt > 1 {
		e.retryAlone = e.active == 1
		want := e.completedWant
		if want == 0 {
			want = 4
		}
		e.allBeforeRetry = len(e.completed) >= want
	}
	fail := p.Request.Query == e.target && attempt <= e.failures
	e.mu.Unlock()
	var arms retrieval.Arms
	var err error
	if fail {
		err = e.cause
	} else {
		arms, err = e.cohortCalibrationEngine.Collect(ctx, p, d)
	}
	e.mu.Lock()
	e.active--
	e.completed[p.Request.Query] = true
	e.mu.Unlock()
	return arms, err
}

func TestCalibrate_RetriesStatementTimeout(t *testing.T) {
	suite := developmentSuite()
	suite.Development[0].Question = "Phát triển một"
	suite.Development[1].Question = "Phát triển hai"
	target := suite.Legacy[0]
	baseline, err := calibrate(context.Background(), &cohortCalibrationEngine{}, suite, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name      string
		failures  int
		cause     error
		wantCalls int
		wantError string
	}{
		{"once", 1, &pgconn.PgError{Code: "57014"}, 2, ""},
		{"twice", 2, &pgconn.PgError{Code: "57014"}, 2, "SQLSTATE 57014"},
		{"other state", 1, &pgconn.PgError{Code: "08006"}, 1, "SQLSTATE 08006"},
		{"other error", 1, errors.New("failure"), 1, "failure"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine := &retryEngine{target: target.Question, failures: tc.failures, cause: tc.cause, calls: map[string]int{}, completed: map[string]bool{}}
			got, err := calibrate(context.Background(), engine, suite, 3)
			if tc.wantError == "" {
				if err != nil || !reflect.DeepEqual(got, baseline) || !engine.retryAlone || !engine.allBeforeRetry {
					t.Fatalf("result=%+v err=%v alone=%t after=%t", got, err, engine.retryAlone, engine.allBeforeRetry)
				}
			} else if err == nil || !strings.Contains(err.Error(), target.ID) || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("error = %v", err)
			}
			if engine.calls[target.Question] != tc.wantCalls {
				t.Fatalf("calls = %v", engine.calls)
			}
		})
	}
}

type modeRetryEngine struct {
	cohortCalibrationEngine
	mu    sync.Mutex
	mode  retrieval.Mode
	calls map[retrieval.Mode]int
}

func (e *modeRetryEngine) Collect(ctx context.Context, p retrieval.Prepared, d retrieval.Depths) (retrieval.Arms, error) {
	e.mu.Lock()
	e.calls[p.Request.Mode]++
	fail := p.Request.Mode == e.mode && e.calls[p.Request.Mode] == 1
	e.mu.Unlock()
	if fail {
		return retrieval.Arms{}, &pgconn.PgError{Code: "57014"}
	}
	return e.cohortCalibrationEngine.Collect(ctx, p, d)
}

func TestPrefetchCases_RetriesExactAndANN(t *testing.T) {
	inputs := []cohortInput{{c: Case{ID: "case", Question: "Câu hỏi tốt"}, cohorts: []string{"old_product"}}}
	modes := []retrieval.Mode{"exact", "ann"}
	for _, mode := range modes {
		t.Run(string(mode), func(t *testing.T) {
			e := &modeRetryEngine{mode: mode, calls: map[retrieval.Mode]int{}}
			results := prefetchCases(context.Background(), e, inputs, nil, modes, e.Release().RetrievalRecipe, ValidityOverrides{}, 3)
			for _, m := range modes {
				result := results[0].collects["old_product"][m]
				if result.err != nil || len(result.arms.Primary.Dense) != 1 {
					t.Fatalf("%s result = %+v", m, result)
				}
				want := 1
				if m == mode {
					want = 2
				}
				if e.calls[m] != want {
					t.Fatalf("%s calls = %d, want %d", m, e.calls[m], want)
				}
			}
		})
	}
}

func TestRun_RetryMatchesUninterruptedSerialRun(t *testing.T) {
	suite := developmentSuite()
	suite.Development[0].Question = "Phát triển một"
	suite.Development[1].Question = "Phát triển hai"
	date := retrieval.Date{Year: 2026, Month: time.November, Day: 9}
	makeEngine := func(failures int) *retryEngine {
		return &retryEngine{cohortCalibrationEngine: cohortCalibrationEngine{fakeEngine: fakeEngine{date: date}}, target: suite.Legacy[0].Question, failures: failures, completedWant: 7, cause: &pgconn.PgError{Code: "57014"}, calls: map[string]int{}, completed: map[string]bool{}}
	}
	opts := Options{EvaluationDate: date}
	baseline, baselineCache, err := run(context.Background(), makeEngine(0), suite, opts, 1)
	if err != nil {
		t.Fatal(err)
	}
	engine := makeEngine(1)
	actual, actualCache, err := run(context.Background(), engine, suite, opts, 3)
	if err != nil {
		t.Fatal(err)
	}
	baseline.TimingsMS, actual.TimingsMS = nil, nil
	if !reflect.DeepEqual(baseline, actual) {
		t.Fatal("report differs from serial run")
	}
	if !reflect.DeepEqual(baselineCache, actualCache) {
		t.Fatal("cache differs from serial run")
	}
	if engine.calls[engine.target] < 2 || !engine.retryAlone || !engine.allBeforeRetry {
		t.Fatalf("retry not isolated after other cases: calls=%v alone=%t after=%t", engine.calls, engine.retryAlone, engine.allBeforeRetry)
	}
}
