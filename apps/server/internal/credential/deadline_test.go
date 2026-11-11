package credential

import (
	"context"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/store"
)

type deadlineStore struct {
	row     store.GetCredentialRow
	bounded bool
}

func (s *deadlineStore) GetCredential(ctx context.Context, _ string) (store.GetCredentialRow, error) {
	deadline, ok := ctx.Deadline()
	s.bounded = ok && time.Until(deadline) > 0 && time.Until(deadline) <= 2*time.Second
	return s.row, nil
}
func (*deadlineStore) TouchPersonalToken(context.Context, string) error { return nil }
func TestVerify_CredentialLookupHasTwoSecondDeadline(t *testing.T) {
	v, original, _, token := fixture(t)
	s := &deadlineStore{row: original.row}
	v.Store = s
	if _, err := v.Verify(context.Background(), token); err != nil {
		t.Fatal(err)
	}
	if !s.bounded {
		t.Fatal("credential lookup lacks the two-second deadline")
	}
}

type timedOutStore struct {
	row   store.GetCredentialRow
	phase string
}

func (s timedOutStore) GetCredential(ctx context.Context, _ string) (store.GetCredentialRow, error) {
	if s.phase == "lookup" {
		<-ctx.Done()
		return store.GetCredentialRow{}, ctx.Err()
	}
	return s.row, nil
}
func (s timedOutStore) TouchPersonalToken(ctx context.Context, _ string) error {
	<-ctx.Done()
	return ctx.Err()
}
func TestVerify_AuthTimeoutReturns503(t *testing.T) {
	for _, phase := range []string{"lookup", "touch"} {
		t.Run(phase, func(t *testing.T) {
			v, s, _, token := fixture(t)
			v.Store = timedOutStore{row: s.row, phase: phase}
			v.QueryTimeout = 5 * time.Millisecond
			started := time.Now()
			w := response(v, []string{"Bearer " + token})
			if w.Code != 503 || time.Since(started) > 100*time.Millisecond || len(v.entries) != 0 {
				t.Fatal("auth timeout did not fail closed with 503", w.Code)
			}
		})
	}
}
