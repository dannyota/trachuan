package app

import (
	"context"
	"fmt"

	"danny.vn/trachuan/apps/server/internal/store"
)

func (s credentialStore) TouchSession(ctx context.Context, hash string) (int64, error) {
	initial, err := s.GetSession(ctx, hash)
	if err != nil {
		return 0, fmt.Errorf("read sliding session: %w", err)
	}
	var count int64
	err = s.withAccount(ctx, initial.AccountID, initial.SessionAuthEpoch, func(q *store.Queries) error {
		var touchErr error
		count, touchErr = q.TouchSession(ctx, hash)
		return touchErr
	})
	return count, err
}
