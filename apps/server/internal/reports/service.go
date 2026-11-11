package reports

import (
	"context"
	"errors"
	"time"

	"danny.vn/trachuan/apps/server/internal/release"
	"danny.vn/trachuan/internal/retrieval"
)

const authDeadline = 2 * time.Second

// Service validates reports and admits them through one locked transaction.
// Configure dependencies before use. Clock defaults to time.Now.
type Service struct {
	Store    TxRunner
	Resolver Resolver
	Clock    func() time.Time
}

// Submit returns an open report only after the admission transaction commits.
func (s *Service) Submit(ctx context.Context, accountID int64, in Input) (Result, error) {
	if accountID <= 0 {
		return Result{}, ErrUnauthenticated
	}
	if err := validateFields(in); err != nil {
		return Result{}, err
	}
	if s.Store == nil || s.Resolver == nil {
		return Result{}, ErrUnavailable
	}
	lease, err := s.Resolver.Resolve(ctx, in.Country, in.Release)
	if err != nil {
		return Result{}, corpusFailure(err)
	}
	if lease.Done != nil {
		defer lease.Done()
	}
	if lease.Release == "" || lease.Corpus == nil {
		return Result{}, ErrUnavailable
	}
	now := time.Now()
	if s.Clock != nil {
		now = s.Clock()
	}
	in.Release = lease.Release
	report, err := Validate(ctx, lease.Corpus, in, now)
	if err != nil {
		return Result{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, authDeadline)
	defer cancel()
	tx, err := s.Store.Begin(ctx)
	if err != nil {
		return Result{}, ErrUnavailable
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.WithoutCancel(ctx), authDeadline)
		defer stop()
		_ = tx.Rollback(cleanup)
	}()
	result, err := admit(ctx, tx, accountID, report, now)
	if err != nil {
		return Result{}, err
	}
	if ctx.Err() != nil {
		return Result{}, ErrUnavailable
	}
	// A failed commit can have an unknown outcome. Never return its report id.
	if err := tx.Commit(ctx); err != nil || ctx.Err() != nil {
		return Result{}, ErrUnavailable
	}
	return result, nil
}

func admit(ctx context.Context, tx Store, accountID int64, report Report, now time.Time) (Result, error) {
	account, err := tx.LockAccountForReport(ctx, LockAccountForReportParams{AccountID: accountID})
	if err != nil {
		return Result{}, ErrUnavailable
	}
	if !account.Exists || account.Blocked || account.DeletionRequested {
		return Result{}, ErrUnauthenticated
	}
	if err = tx.LockReports(ctx); err != nil {
		return Result{}, ErrUnavailable
	}
	counts, err := tx.CountAccountReports(ctx, CountAccountReportsParams{AccountID: accountID, Since: now.Add(-24 * time.Hour)})
	if err != nil {
		return Result{}, ErrUnavailable
	}
	retained, err := tx.CountRetainedReports(ctx)
	if err != nil {
		return Result{}, ErrUnavailable
	}
	result, found, err := tx.FindOpenReport(ctx, FindOpenReportParams{AccountID: accountID, GroupKey: report.GroupKey})
	if err != nil {
		return Result{}, ErrUnavailable
	}
	if found {
		result.Deduplicated = true
		return result, nil
	}
	if counts.Daily >= 20 || counts.Open >= 200 || counts.Retained >= 1000 || retained >= 100000 {
		var delay time.Duration
		if counts.Daily >= 20 {
			oldest, oldestErr := tx.OldestAccountReport(ctx, CountAccountReportsParams{AccountID: accountID, Since: now.Add(-24 * time.Hour)})
			if oldestErr != nil || ctx.Err() != nil {
				return Result{}, ErrUnavailable
			}
			delay = oldest.Add(24 * time.Hour).Sub(now)
		}
		if counts.Open >= 200 || counts.Retained >= 1000 || retained >= 100000 {
			delay = max(delay, time.Hour)
		}
		return Result{}, RateLimitError{RetryAfter: delay}
	}
	result, err = tx.InsertReport(ctx, InsertReportParams{AccountID: accountID, Report: report, CreatedAt: now})
	if err != nil {
		return Result{}, ErrUnavailable
	}
	result.Deduplicated = false
	return result, nil
}

func corpusFailure(err error) error {
	if errors.Is(err, ErrReleaseExpired) || errors.Is(err, release.ErrExpired) || errors.Is(err, retrieval.ErrReleaseGone) {
		return ErrReleaseExpired
	}
	// Causes can contain request text, SQL or connection details.
	return ErrUnavailable
}
