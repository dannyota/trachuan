package reports

import (
	"context"
	"errors"
	"sync"
	"time"
)

type fakeResolver struct {
	err       error
	corpus    Corpus
	requested string
	country   string
	done      int
	release   string
}

func (r *fakeResolver) Resolve(_ context.Context, country, requested string) (Lease, error) {
	r.country, r.requested = country, requested
	c := r.corpus
	if c == nil {
		c = fakeCorpus{}
	}
	active := r.release
	if active == "" {
		active = "20261111"
	}
	if r.err == nil && requested != "" && requested != active {
		return Lease{}, ErrReleaseExpired
	}
	return Lease{Corpus: c, Release: active, Done: func() { r.done++ }}, r.err
}

type fakeRunner struct {
	mu                         sync.Mutex
	account                    AccountState
	counts                     AccountCounts
	retained                   int64
	rows                       []InsertReportParams
	fail                       string
	events                     []string
	begins, commits, rollbacks int
	since                      time.Time
	accountID                  int64
	deadline                   bool
	commitUnknown              bool
	cancelAt                   string
	cancel                     context.CancelFunc
	rollbackCanceled           bool
}

func (r *fakeRunner) step(ctx context.Context, name string) error {
	r.events = append(r.events, name)
	if name == r.cancelAt && r.cancel != nil {
		r.cancel()
	}
	if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > 2*time.Second {
		r.deadline = false
	}
	if r.fail == name {
		return errors.New("PRIVATE database detail")
	}
	return nil
}

func (r *fakeRunner) Begin(ctx context.Context) (Transaction, error) {
	r.mu.Lock()
	r.begins++
	if err := r.step(ctx, "begin"); err != nil {
		r.mu.Unlock()
		return nil, err
	}
	return &fakeTx{r: r}, nil
}

type fakeTx struct {
	r       *fakeRunner
	pending *InsertReportParams
	ended   bool
}

func (tx *fakeTx) LockAccountForReport(ctx context.Context, p LockAccountForReportParams) (AccountState, error) {
	tx.r.accountID = p.AccountID
	return tx.r.account, tx.r.step(ctx, "account")
}
func (tx *fakeTx) LockReports(ctx context.Context) error { return tx.r.step(ctx, "lock") }
func (tx *fakeTx) CountAccountReports(ctx context.Context, p CountAccountReportsParams) (AccountCounts, error) {
	tx.r.since = p.Since
	counts := tx.r.counts
	for _, row := range tx.r.rows {
		if row.AccountID == p.AccountID {
			counts.Open++
			counts.Retained++
			if !row.CreatedAt.Before(p.Since) {
				counts.Daily++
			}
		}
	}
	return counts, tx.r.step(ctx, "counts")
}
func (tx *fakeTx) CountRetainedReports(ctx context.Context) (int64, error) {
	return tx.r.retained + int64(len(tx.r.rows)), tx.r.step(ctx, "retained")
}
func (tx *fakeTx) FindOpenReport(ctx context.Context, p FindOpenReportParams) (Result, bool, error) {
	err := tx.r.step(ctx, "duplicate")
	for i, row := range tx.r.rows {
		if row.AccountID == p.AccountID && row.Report.GroupKey == p.GroupKey {
			return Result{ReportID: int64(i + 1), State: "open"}, true, err
		}
	}
	return Result{}, false, err
}
func (tx *fakeTx) InsertReport(ctx context.Context, p InsertReportParams) (Result, error) {
	tx.pending = &p
	return Result{ReportID: int64(len(tx.r.rows) + 1), State: "open"}, tx.r.step(ctx, "insert")
}
func (tx *fakeTx) Commit(ctx context.Context) error {
	tx.r.commits++
	err := tx.r.step(ctx, "commit")
	if err == nil && tx.pending != nil {
		tx.r.rows = append(tx.r.rows, *tx.pending)
	}
	if tx.r.commitUnknown {
		err = errors.New("PRIVATE commit outcome unknown")
	}
	tx.ended = true
	tx.r.mu.Unlock()
	return err
}
func (tx *fakeTx) Rollback(ctx context.Context) error {
	if tx.ended {
		return nil
	}
	tx.r.rollbacks++
	tx.r.rollbackCanceled = ctx.Err() != nil
	err := tx.r.step(ctx, "rollback")
	tx.ended = true
	tx.r.mu.Unlock()
	return err
}

func serviceFixture() (*Service, *fakeRunner, *fakeResolver) {
	r := &fakeRunner{account: AccountState{Exists: true}, deadline: true}
	resolver := &fakeResolver{}
	return &Service{Store: r, Resolver: resolver, Clock: testNow}, r, resolver
}
