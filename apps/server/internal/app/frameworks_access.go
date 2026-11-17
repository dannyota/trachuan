package app

import (
	"context"

	"danny.vn/trachuan/apps/server/internal/frameworkaccess"
	"danny.vn/trachuan/apps/server/internal/store"
	frameworkserve "danny.vn/trachuan/internal/frameworks/serve"
)

type entitlementFallback struct {
	frameworkaccess.Store
	unavailable bool
}

func (s *entitlementFallback) ListLiveFrameworkEntitlementsByEmail(ctx context.Context, email string) ([]store.FrameworkEntitlement, error) {
	rows, err := s.Store.ListLiveFrameworkEntitlementsByEmail(ctx, email)
	if err != nil {
		s.unavailable = true
		return nil, nil
	}
	return rows, nil
}

func frameworkCaller(auth frameworkaccess.Store) func(context.Context, int64, []string) (frameworkserve.Caller, error) {
	return func(ctx context.Context, id int64, codes []string) (frameworkserve.Caller, error) {
		ctx, cancel := context.WithTimeout(ctx, authQueryDeadline)
		defer cancel()
		fallback := &entitlementFallback{Store: auth}
		access := frameworkaccess.Service{Store: fallback, Frameworks: codes}
		set, err := access.AccessSet(ctx, id)
		if err != nil {
			return frameworkserve.Caller{}, err
		}
		return frameworkserve.Caller{Readable: func(code, class string) bool {
			return frameworkaccess.Decide(code, class, set) == frameworkaccess.Full
		}, AccessUnavailable: fallback.unavailable}, nil
	}
}
