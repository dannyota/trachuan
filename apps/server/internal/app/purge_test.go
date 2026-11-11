package app

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/store"
)

type failingPurgeStore struct {
	mu                          sync.Mutex
	login, account, credentials int
	done                        chan struct{}
}

func (s *failingPurgeStore) DeleteExpiredLoginTransactions(context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.login++
	return errors.New("private driver detail")
}
func (s *failingPurgeStore) PurgeDeletedAccounts(context.Context, time.Time) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.account++
	s.done <- struct{}{}
	return 0, errors.New("private driver detail")
}
func (s *failingPurgeStore) PurgeEndedCredentials(context.Context) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.credentials++
	return 0, errors.New("private driver detail")
}
func (*failingPurgeStore) DeleteExpiredReports(context.Context, store.DeleteExpiredReportsParams) (int64, error) {
	return 0, errors.New("private driver detail")
}
func (*failingPurgeStore) DeleteExpiredSnapshots(context.Context, store.DeleteExpiredSnapshotsParams) (int64, error) {
	return 0, errors.New("private driver detail")
}
func TestPurge_LogsRetriesAndStops(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	s := &failingPurgeStore{done: make(chan struct{}, 2)}
	ticks := make(chan time.Time)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); runPurgeLoop(ctx, s, time.Now, logger, ticks) }()
	for i := 0; i < 2; i++ {
		ticks <- time.Now()
		<-s.done
	}
	cancel()
	<-done
	s.mu.Lock()
	login, account, credentials := s.login, s.account, s.credentials
	s.mu.Unlock()
	if login != 2 || account != 2 || credentials != 2 {
		t.Fatalf("purge calls login=%d account=%d", login, account)
	}
	out := logs.String()
	if strings.Count(out, `"msg":"purge failed"`) != 10 || strings.Count(out, `"step":"reports"`) != 2 || strings.Count(out, `"step":"report_snapshots"`) != 2 || strings.Count(out, `"step":"login_transactions"`) != 2 || strings.Count(out, `"step":"deleted_accounts"`) != 2 || strings.Count(out, `"step":"ended_credentials"`) != 2 || strings.Contains(out, "private driver detail") {
		t.Fatal("unexpected purge events", out)
	}
}
