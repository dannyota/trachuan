package app

import (
	"context"
	"errors"
	"testing"

	"danny.vn/trachuan/apps/server/internal/store"
)

type frameworkAccessStore struct {
	accountErr, entitlementErr error
	entitlementCalls           int
}

func (s *frameworkAccessStore) GetAccount(context.Context, int64) (store.Account, error) {
	return store.Account{ID: 7, Email: "invented@example.test"}, s.accountErr
}

func (s *frameworkAccessStore) ListLiveFrameworkEntitlementsByEmail(context.Context, string) ([]store.FrameworkEntitlement, error) {
	s.entitlementCalls++
	return []store.FrameworkEntitlement{{Email: "invented@example.test", Scope: "fw-alpha"}}, s.entitlementErr
}

func TestFrameworkAccess_OnlyEntitlementFailureFallsBack(t *testing.T) {
	s := &frameworkAccessStore{entitlementErr: errors.New("invented entitlement failure")}
	access := frameworkCaller(s)
	caller, err := access(t.Context(), 7, []string{"fw-alpha", "fw-beta"})
	if err != nil || !caller.AccessUnavailable || caller.Readable("fw-alpha", "licensed") || !caller.Readable("fw-beta", "free") {
		t.Fatalf("incorrect entitlement fallback: %v", err)
	}
	s.entitlementErr = nil
	caller, err = access(t.Context(), 7, []string{"fw-alpha", "fw-beta"})
	if err != nil || caller.AccessUnavailable || !caller.Readable("fw-alpha", "licensed") || caller.Readable("fw-unknown", "free") || s.entitlementCalls != 2 {
		t.Fatalf("access was cached or incorrectly projected: %v", err)
	}
	s.accountErr = errors.New("invented account store failure")
	if _, err = access(t.Context(), 7, []string{"fw-alpha"}); err == nil || s.entitlementCalls != 2 {
		t.Fatal("account store failure fell back to reduced access")
	}
}
