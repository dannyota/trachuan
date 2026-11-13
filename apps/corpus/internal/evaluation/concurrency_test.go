package evaluation

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"danny.vn/trachuan/internal/retrieval"
)

type concurrentEngine struct {
	fakeEngine
	active   atomic.Int32
	peak     atomic.Int32
	failures map[string]bool
}

func (e *concurrentEngine) Prepare(ctx context.Context, r retrieval.Request) (retrieval.Prepared, error) {
	return (&fakeEngine{}).Prepare(ctx, r)
}

func (e *concurrentEngine) PrepareLegacy(ctx context.Context, r retrieval.Request) (retrieval.Prepared, error) {
	return (&fakeEngine{}).PrepareLegacy(ctx, r)
}

func (e *concurrentEngine) Collect(ctx context.Context, p retrieval.Prepared, d retrieval.Depths) (retrieval.Arms, error) {
	n := e.active.Add(1)
	defer e.active.Add(-1)
	for old := e.peak.Load(); n > old; old = e.peak.Load() {
		if e.peak.CompareAndSwap(old, n) {
			break
		}
	}
	delay := 2 * time.Millisecond
	if p.Request.Query == "Câu hỏi tốt" {
		delay = 12 * time.Millisecond
	}
	select {
	case <-time.After(delay):
	case <-ctx.Done():
		return retrieval.Arms{}, ctx.Err()
	}
	if e.failures[p.Request.Query] {
		return retrieval.Arms{}, errors.New("injected failure: " + p.Request.Query)
	}
	return retrieval.Arms{Validation: p.Validation, Depths: d, Date: e.date}, nil
}

func TestRun_CollectsCasesConcurrentlyWithinLimit(t *testing.T) {
	date := retrieval.Date{Year: 2026, Month: time.November, Day: 9}
	engine := &concurrentEngine{fakeEngine: fakeEngine{date: date}}
	_, _, err := Run(context.Background(), engine, recordedSuite(), Options{EvaluationDate: date, Modes: []retrieval.Mode{"exact"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := engine.peak.Load(); got != 3 {
		t.Fatalf("peak concurrent collects = %d, want 3", got)
	}
}

func TestCalibrate_CollectsCasesConcurrentlyWithinLimit(t *testing.T) {
	date := retrieval.Date{Year: 2026, Month: time.November, Day: 9}
	engine := &concurrentEngine{fakeEngine: fakeEngine{date: date}}
	_, err := Calibrate(context.Background(), engine, recordedSuite())
	if err != nil {
		t.Fatal(err)
	}
	if got := engine.peak.Load(); got != 2 {
		t.Fatalf("peak concurrent collects = %d, want 2", got)
	}
}

type lexicalReuseEngine struct {
	concurrentEngine
	reused  atomic.Int32
	invalid atomic.Bool
}

func (e *lexicalReuseEngine) Collect(ctx context.Context, p retrieval.Prepared, d retrieval.Depths) (retrieval.Arms, error) {
	a, err := e.concurrentEngine.Collect(ctx, p, d)
	hit := lossCandidate(123, "doc", "one")
	hit.BM25, hit.Rank = 4, 1
	a.Primary.Lexical = []retrieval.Candidate{hit}
	a.Primary.Members = []retrieval.Candidate{hit}
	return a, err
}

func (e *lexicalReuseEngine) CollectWithLexical(ctx context.Context, p retrieval.Prepared, d retrieval.Depths, lexical retrieval.LexicalArms) (retrieval.Arms, error) {
	e.reused.Add(1)
	if len(lexical.Primary) != 1 || lexical.Primary[0].ChunkID != 123 || lexical.Depth != 100 {
		e.invalid.Store(true)
	}
	return e.Collect(ctx, p, d)
}

func (e *lexicalReuseEngine) CollectWithNotYetEligible(ctx context.Context, p retrieval.Prepared, d retrieval.Depths) (retrieval.Arms, error) {
	return e.Collect(ctx, p, d)
}

func TestRun_ReusesExactLexicalForANN(t *testing.T) {
	date := retrieval.Date{Year: 2026, Month: time.November, Day: 9}
	for _, tc := range []struct {
		name   string
		modes  []retrieval.Mode
		notYet bool
		want   bool
	}{
		{"exact then ann", []retrieval.Mode{"exact", "ann"}, false, true},
		{"ann only", []retrieval.Mode{"ann"}, false, false},
		{"ann first", []retrieval.Mode{"ann", "exact"}, false, false},
		{"not yet", []retrieval.Mode{"exact", "ann"}, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine := &lexicalReuseEngine{concurrentEngine: concurrentEngine{fakeEngine: fakeEngine{date: date}}}
			_, _, err := run(context.Background(), engine, recordedSuite(), Options{EvaluationDate: date, Modes: tc.modes, ValidityOverrides: ValidityOverrides{NotYetEligible: tc.notYet}}, 1)
			if err != nil {
				t.Fatal(err)
			}
			if got := engine.reused.Load() > 0; got != tc.want || engine.invalid.Load() {
				t.Fatalf("reused=%t invalid=%t", got, engine.invalid.Load())
			}
		})
	}
}

func TestCasePools_PreserveOrderAndStopOnError(t *testing.T) {
	date := retrieval.Date{Year: 2026, Month: time.November, Day: 9}
	suite := recordedSuite()
	opts := Options{EvaluationDate: date}
	makeEngine := func() *concurrentEngine { return &concurrentEngine{fakeEngine: fakeEngine{date: date}} }
	serial, serialCache, err := run(context.Background(), makeEngine(), suite, opts, 1)
	if err != nil {
		t.Fatal(err)
	}
	parallel, parallelCache, err := run(context.Background(), makeEngine(), suite, opts, 3)
	if err != nil {
		t.Fatal(err)
	}
	serial.TimingsMS, parallel.TimingsMS = nil, nil
	dirs := []string{t.TempDir(), t.TempDir()}
	for i, pair := range []struct {
		report Report
		cache  Cache
	}{{serial, serialCache}, {parallel, parallelCache}} {
		if writeErr := pair.report.WriteFiles(dirs[i]); writeErr != nil {
			t.Fatal(writeErr)
		}
		if writeErr := pair.cache.WriteFiles(dirs[i]); writeErr != nil {
			t.Fatal(writeErr)
		}
	}
	for _, name := range []string{"report.json", "cases.jsonl", "arms.jsonl", "queries.f32", "query-index.json"} {
		a, readErr := os.ReadFile(filepath.Join(dirs[0], name))
		if readErr != nil {
			t.Fatal(readErr)
		}
		b, readErr := os.ReadFile(filepath.Join(dirs[1], name))
		if readErr != nil {
			t.Fatal(readErr)
		}
		if !bytes.Equal(a, b) {
			t.Errorf("%s differs", name)
		}
	}
	serialCal, err := calibrate(context.Background(), makeEngine(), suite, 1)
	if err != nil {
		t.Fatal(err)
	}
	parallelCal, err := calibrate(context.Background(), makeEngine(), suite, 3)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(serialCal, parallelCal) {
		t.Fatal("calibration differs")
	}
	for _, calibration := range []bool{false, true} {
		errorSuite := developmentSuite()
		var errorsByLimit []string
		for _, limit := range []int{1, 3} {
			e := makeEngine()
			e.failures = map[string]bool{errorSuite.Legacy[1].Question: true, errorSuite.Development[0].Question: true}
			var err error
			if calibration {
				_, err = calibrate(context.Background(), e, errorSuite, limit)
			} else {
				_, _, err = run(context.Background(), e, errorSuite, opts, limit)
			}
			if err == nil {
				t.Fatal("expected failure")
			}
			errorsByLimit = append(errorsByLimit, err.Error())
			if e.active.Load() != 0 {
				t.Fatal("worker still active")
			}
		}
		if errorsByLimit[0] != errorsByLimit[1] {
			t.Fatalf("error order differs: %v", errorsByLimit)
		}
	}
}

type noLexicalReuseEngine struct {
	concurrentEngine
	annCalls atomic.Int32
}

func (e *noLexicalReuseEngine) Collect(ctx context.Context, p retrieval.Prepared, d retrieval.Depths) (retrieval.Arms, error) {
	if p.Request.Mode == "ann" {
		e.annCalls.Add(1)
	}
	return e.concurrentEngine.Collect(ctx, p, d)
}

func TestRun_UsesCollectWhenLexicalReuseUnsupported(t *testing.T) {
	date := retrieval.Date{Year: 2026, Month: time.November, Day: 9}
	engine := &noLexicalReuseEngine{concurrentEngine: concurrentEngine{fakeEngine: fakeEngine{date: date}}}
	_, _, err := run(context.Background(), engine, recordedSuite(), Options{EvaluationDate: date}, 3)
	if err != nil {
		t.Fatal(err)
	}
	if got := engine.annCalls.Load(); got != 7 {
		t.Fatalf("ann Collect calls = %d, want 7", got)
	}
}
