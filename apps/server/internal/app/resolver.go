package app

import (
	"context"
	"errors"

	"danny.vn/trachuan/apps/server/internal/mcpapi"
	"danny.vn/trachuan/apps/server/internal/release"
	"danny.vn/trachuan/apps/server/internal/store"
	"github.com/jackc/pgx/v5"
)

type activeStore struct{ queries *store.Queries }

func (s activeStore) GetActiveRelease(ctx context.Context, country string) (string, error) {
	row, err := s.queries.GetActiveRelease(ctx, country)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return row.ActiveDb, err
}

type mcpResolver struct{ resolver *release.Resolver }

func (r mcpResolver) Resolve(ctx context.Context, country, rel string) (mcpapi.Engine, string, func(), error) {
	lease, err := r.resolver.Resolve(ctx, country, rel)
	if err != nil {
		return nil, "", nil, err
	}
	return lease.Engine, lease.Release, lease.Done, nil
}
