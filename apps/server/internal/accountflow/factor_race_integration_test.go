//go:build integration

package accountflow

import (
	"errors"
	"sync"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/factor"
	"danny.vn/trachuan/apps/server/internal/store"
)

func TestFactor_ConcurrentPendingRowsAcceptStepOnce(t *testing.T) {
	s, db, a := factorFixture(t)
	a, _, secret := enroll(t, s, a)
	q := store.New(db)
	pending := make([]store.CreateSessionParams, 2)
	for i := range pending {
		pending[i] = factorSessionParams()
		_, err := q.CreateGoogleSession(t.Context(), store.GoogleSessionParams{Identity: store.GoogleIdentity{Sub: "factor"}, Session: pending[i]})
		var required *store.PendingRequired
		if !errors.As(err, &required) {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(t.Context(), `UPDATE totp_credential SET last_step=NULL WHERE account_id=$1`, a.AccountID); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	code := factor.Code(secret, time.Now().Unix()/30)
	for _, p := range pending {
		wg.Go(func() {
			_, _, err := s.CompleteFactor(t.Context(), FactorCompletion{TokenHash: p.TokenHash, CSRF: p.CsrfSecret, Code: code, Session: factorSessionParams()})
			results <- err
		})
	}
	wg.Wait()
	close(results)
	accepted := 0
	for err := range results {
		if err == nil {
			accepted++
		} else if !errors.Is(err, factor.ErrFailed) {
			t.Fatal(err)
		}
	}
	if accepted != 1 {
		t.Fatal("one step completed multiple pending rows")
	}
	var count, failures int
	if err := db.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM pending_auth WHERE account_id=$1 AND consumed_at IS NOT NULL),failures FROM totp_credential WHERE account_id=$1`, a.AccountID).Scan(&count, &failures); err != nil || count != 1 || failures != 1 {
		t.Fatal("replay side effects", err)
	}
}
func TestFactor_PendingSupersessionAndEpochFence(t *testing.T) {
	s, db, a := factorFixture(t)
	a, _, _ = enroll(t, s, a)
	q := store.New(db)
	first := factorSessionParams()
	var required *store.PendingRequired
	if _, err := q.CreateGoogleSession(t.Context(), store.GoogleSessionParams{Identity: store.GoogleIdentity{Sub: "factor"}, Session: first}); !errors.As(err, &required) {
		t.Fatal(err)
	}
	second := factorSessionParams()
	if _, err := q.CreateGoogleSession(t.Context(), store.GoogleSessionParams{Identity: store.GoogleIdentity{Sub: "factor"}, Session: second, Pending: store.PendingParams{ReplacedHashes: []string{first.TokenHash}}}); !errors.As(err, &required) {
		t.Fatal(err)
	}
	if _, err := s.PendingFactor(t.Context(), first.TokenHash); !errors.Is(err, factor.ErrFailed) {
		t.Fatal("superseded pending row is live", err)
	}
	if _, err := s.PendingFactor(t.Context(), second.TokenHash); err != nil {
		t.Fatal(err)
	}
	if _, err := q.BlockAccount(t.Context(), a.AccountID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.CompleteFactor(t.Context(), FactorCompletion{TokenHash: second.TokenHash, CSRF: second.CsrfSecret, Code: "wrong", Session: factorSessionParams()}); !errors.Is(err, factor.ErrFailed) {
		t.Fatal("pending survived epoch change", err)
	}
}
