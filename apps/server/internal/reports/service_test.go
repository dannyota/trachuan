package reports

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestSubmit_DedupeAndCaps(t *testing.T) {
	t.Run("duplicate preserves original across releases and suggestions at caps", func(t *testing.T) {
		svc, runner, resolver := serviceFixture()
		in := validInput("wrong_validity")
		in.Release = "20261111"
		in.Suggestion = suggestion(map[string]string{"status_class": "partial"})
		first, err := svc.Submit(context.Background(), 42, in)
		if err != nil || first.ReportID != 1 || first.State != "open" || first.Deduplicated {
			t.Fatalf("first = %+v, err = %v", first, err)
		}
		runner.counts = AccountCounts{Daily: 20, Open: 200, Retained: 1000}
		runner.retained = 100000
		in.Release = ""
		resolver.release = "20261112"
		in.Suggestion = suggestion(map[string]string{"status_class": "expired"})
		second, err := svc.Submit(context.Background(), 42, in)
		if err != nil || second.ReportID != first.ReportID || !second.Deduplicated || second.State != "open" {
			t.Fatalf("repeat = %+v, err = %v", second, err)
		}
		if len(runner.rows) != 1 || runner.rows[0].Report.Suggestion.StatusClass != "partial" || runner.rows[0].Report.Release != "20261111" {
			t.Fatalf("stored rows = %+v", runner.rows)
		}
		if resolver.done != 2 || runner.commits != 2 {
			t.Fatalf("done=%d commits=%d", resolver.done, runner.commits)
		}
		if resolver.requested != "" {
			t.Fatal("default release was not resolved")
		}
	})
	for _, tc := range []struct {
		name   string
		counts AccountCounts
		total  int64
	}{
		{"daily", AccountCounts{Daily: 20}, 0}, {"open", AccountCounts{Open: 200}, 0},
		{"retained including withdrawn", AccountCounts{Retained: 1000}, 0}, {"global including snapshots", AccountCounts{}, 100000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, runner, resolver := serviceFixture()
			runner.counts, runner.retained = tc.counts, tc.total
			_, err := svc.Submit(context.Background(), 42, validInput("wrong_text"))
			requireCode(t, err, ErrRateLimited)
			if len(runner.rows) != 0 || runner.commits != 0 || runner.rollbacks != 1 || resolver.done != 1 {
				t.Fatalf("admission was not rolled back: %+v", runner)
			}
		})
	}
	t.Run("last slot and transaction order", func(t *testing.T) {
		svc, runner, resolver := serviceFixture()
		runner.counts = AccountCounts{Daily: 19, Open: 199, Retained: 999}
		runner.retained = 99999
		in := validInput("missing_document")
		in.Suggestion = suggestion(map[string]string{"document_number": " 1 / 2025 / tt - nhnn "})
		_, err := svc.Submit(context.Background(), 42, in)
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"begin", "account", "lock", "counts", "retained", "duplicate", "insert", "commit"}
		if !reflect.DeepEqual(runner.events, want) {
			t.Fatalf("events = %v", runner.events)
		}
		if !runner.deadline || runner.accountID != 42 || !runner.since.Equal(testNow().Add(-24*time.Hour)) {
			t.Fatalf("admission bounds: %+v", runner)
		}
		row := runner.rows[0]
		if row.Report.Target != "1/2025/TT-NHNN" || row.AccountID != 42 || !row.CreatedAt.Equal(testNow()) || resolver.country != "vn" {
			t.Fatalf("row = %+v", row)
		}
	})
	t.Run("different account and target do not deduplicate", func(t *testing.T) {
		svc, runner, _ := serviceFixture()
		for _, tc := range []struct {
			account int64
			number  string
		}{{1, "1/2025/TT"}, {2, "1/2025/TT"}, {1, "2/2025/TT"}} {
			in := validInput("missing_document")
			in.Suggestion = suggestion(map[string]string{"document_number": tc.number})
			result, err := svc.Submit(context.Background(), tc.account, in)
			if err != nil || result.Deduplicated {
				t.Fatalf("result = %+v, err = %v", result, err)
			}
		}
		if len(runner.rows) != 3 {
			t.Fatal("reports collapsed across accounts or targets")
		}
	})
}

func TestSubmit_UnknownCommitOutcome(t *testing.T) {
	svc, runner, _ := serviceFixture()
	runner.commitUnknown = true
	result, err := svc.Submit(context.Background(), 1, validInput("wrong_text"))
	requireCode(t, err, ErrUnavailable)
	if result != (Result{}) || len(runner.rows) != 1 {
		t.Fatalf("unknown commit returned a report or did not persist in fake: %+v", result)
	}
	runner.commitUnknown = false
	result, err = svc.Submit(context.Background(), 1, validInput("wrong_text"))
	if err != nil || !result.Deduplicated || result.ReportID != 1 || len(runner.rows) != 1 {
		t.Fatalf("retry after unknown commit = %+v, err = %v", result, err)
	}
}

func TestSubmit_CancellationRollsBack(t *testing.T) {
	svc, runner, resolver := serviceFixture()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner.cancelAt, runner.cancel = "insert", cancel
	result, err := svc.Submit(ctx, 1, validInput("wrong_text"))
	requireCode(t, err, ErrUnavailable)
	if result != (Result{}) || len(runner.rows) != 0 || runner.commits != 0 || runner.rollbacks != 1 || runner.rollbackCanceled || resolver.done != 1 {
		t.Fatalf("canceled admission did not clean up: %+v", runner)
	}
}

func TestSubmit_ReleasePinning(t *testing.T) {
	for _, requested := range []string{"20261110", "PRIVATE"} {
		svc, runner, resolver := serviceFixture()
		in := validInput("wrong_text")
		in.Release = requested
		_, err := svc.Submit(context.Background(), 1, in)
		requireCode(t, err, ErrReleaseExpired)
		if runner.begins != 0 || resolver.requested != requested {
			t.Fatal("nonactive release bypassed resolver or began admission")
		}
	}
}

func TestSubmit_BlockedAccount(t *testing.T) {
	for _, state := range []AccountState{{Exists: true, Blocked: true}, {Exists: true, DeletionRequested: true}, {}} {
		svc, runner, _ := serviceFixture()
		runner.account = state
		_, err := svc.Submit(context.Background(), 42, validInput("wrong_text"))
		requireCode(t, err, ErrUnauthenticated)
		if !reflect.DeepEqual(runner.events, []string{"begin", "account", "rollback"}) {
			t.Fatalf("blocked account reached report queries: %v", runner.events)
		}
	}
	svc, runner, _ := serviceFixture()
	_, err := svc.Submit(context.Background(), 0, validInput("wrong_text"))
	requireCode(t, err, ErrUnauthenticated)
	if runner.begins != 0 {
		t.Fatal("invalid account began transaction")
	}
}

func TestSubmit_FailuresCloseAdmission(t *testing.T) {
	for _, stage := range []string{"begin", "account", "lock", "counts", "retained", "duplicate", "insert", "commit"} {
		t.Run(stage, func(t *testing.T) {
			svc, runner, resolver := serviceFixture()
			runner.fail = stage
			result, err := svc.Submit(context.Background(), 1, validInput("wrong_text"))
			requireCode(t, err, ErrUnavailable)
			if result != (Result{}) || len(runner.rows) != 0 || resolver.done != 1 {
				t.Fatalf("failed admission returned success: %+v", result)
			}
			if stage != "begin" && stage != "commit" && runner.rollbacks != 1 {
				t.Fatal("transaction was not rolled back")
			}
		})
	}
	for _, cause := range []error{errors.New("PRIVATE"), context.DeadlineExceeded} {
		svc, runner, resolver := serviceFixture()
		resolver.err = cause
		_, err := svc.Submit(context.Background(), 1, validInput("wrong_text"))
		requireCode(t, err, ErrUnavailable)
		if runner.begins != 0 {
			t.Fatal("resolver failure began transaction")
		}
	}
	for _, kind := range []string{"wrong_text", "broken_citation", "missing_document"} {
		svc, runner, resolver := serviceFixture()
		resolver.corpus = fakeCorpus{err: errors.New("PRIVATE")}
		in := validInput(kind)
		if kind == "broken_citation" {
			in.Suggestion = suggestion(map[string]string{"document_number": "196-HĐBT"})
		}
		_, err := svc.Submit(context.Background(), 1, in)
		requireCode(t, err, ErrUnavailable)
		if runner.begins != 0 || resolver.done != 1 {
			t.Fatal("corpus failure leaked lease or began transaction")
		}
	}
	for _, bad := range []Service{{}, {Store: &fakeRunner{}}, {Resolver: &fakeResolver{}}} {
		_, err := bad.Submit(context.Background(), 1, validInput("wrong_text"))
		requireCode(t, err, ErrUnavailable)
	}
}

func TestSubmit_ConcurrentAdmission(t *testing.T) {
	// The fake serializes whole transactions, as the account and reports locks
	// must do in the adapter. This checks the service keeps counts inside them.
	for _, same := range []bool{true, false} {
		svc, runner, _ := serviceFixture()
		// A resolver per call keeps its test-only observation fields independent.
		var wg sync.WaitGroup
		results := make(chan Result, 30)
		failures := make(chan error, 30)
		for i := range 30 {
			wg.Go(func() {
				local := *svc
				local.Resolver = &fakeResolver{}
				in := validInput("missing_document")
				if !same {
					in.Suggestion = suggestion(map[string]string{"document_number": time.Date(2000+i, 1, 1, 0, 0, 0, 0, time.UTC).Format("06") + "/2025/TT"})
				}
				result, err := local.Submit(context.Background(), 1, in)
				if err != nil {
					failures <- err
				} else {
					results <- result
				}
			})
		}
		wg.Wait()
		close(results)
		close(failures)
		want := 20
		if same {
			want = 1
		}
		if len(runner.rows) != want {
			t.Fatalf("same=%v rows=%d want=%d", same, len(runner.rows), want)
		}
		if same && len(results) != 30 {
			t.Fatal("identical requests were capped")
		}
		for err := range failures {
			requireCode(t, err, ErrRateLimited)
		}
	}
}

func TestSubmit_DailyCapOldestFailure(t *testing.T) {
	svc, runner, _ := serviceFixture()
	runner.counts.Daily = 20
	runner.fail = "oldest"
	result, err := svc.Submit(t.Context(), 1, validInput("wrong_text"))
	requireCode(t, err, ErrUnavailable)
	if result != (Result{}) || runner.commits != 0 || runner.rollbacks != 1 {
		t.Fatalf("result=%+v runner=%+v", result, runner)
	}
}
