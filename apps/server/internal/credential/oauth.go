package credential

import (
	"context"

	"danny.vn/trachuan/apps/server/internal/store"
)

type oauthCredentials interface {
	GetToken(context.Context, string) (store.GetTokenRow, error)
	TouchOAuthGrant(context.Context, int64) error
}

func (v *Verifier) lookup(ctx context.Context, raw, hash string) (store.GetCredentialRow, int64, string, func(context.Context) error, error) {
	if raw[:4] == "trc_" {
		row, err := v.Store.GetCredential(ctx, hash)
		return row, 0, "mcp:read", func(ctx context.Context) error { return v.Store.TouchPersonalToken(ctx, hash) }, err
	}
	db, ok := v.Store.(oauthCredentials)
	if !ok {
		return store.GetCredentialRow{}, 0, "", nil, ErrUnavailable
	}
	row, err := db.GetToken(ctx, hash)
	if err != nil {
		return store.GetCredentialRow{}, 0, "", nil, err
	}
	if row.Kind != "access" || row.GrantRevokedAt != nil || row.DeleteRequestedAt != nil || row.Resource != v.Audience || !row.FamilyExpiresAt.After(v.now()) {
		return store.GetCredentialRow{}, 0, "", nil, ErrUnauthorized
	}
	return store.GetCredentialRow{AccountID: row.AccountID, AccountAuthEpoch: row.AccountAuthEpoch, TokenAuthEpoch: row.AuthEpoch, RevokedAt: row.RevokedAt, BlockedAt: row.BlockedAt, ExpiresAt: minTime(row.ExpiresAt, row.FamilyExpiresAt)}, row.GrantID, row.Scope, func(ctx context.Context) error { return db.TouchOAuthGrant(ctx, row.GrantID) }, nil
}

// EvictGrant removes cached access tokens for a grant and fences old lookups.
func (v *Verifier) EvictGrant(id int64) {
	v.mu.Lock()
	defer v.mu.Unlock()
	for _, e := range v.entries {
		if e.Value.(cacheEntry).grantID == id {
			v.remove(e)
		}
	}
	if len(v.flights) > 0 {
		v.generation++
		if v.grantGenerations == nil {
			v.grantGenerations = make(map[int64]uint64)
		}
		v.grantGenerations[id] = v.generation
	}
}
