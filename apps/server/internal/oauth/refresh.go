package oauth

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/url"
	"time"

	"danny.vn/trachuan/apps/server/internal/store"
	"github.com/jackc/pgx/v5"
)

func (h *tokenHandler) refresh(ctx context.Context, tx pgx.Tx, form url.Values) (tokenResponse, int64, string, error) {
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(form.Get("refresh_token"))))
	q := store.New(tx)
	initial, err := q.GetToken(ctx, hash)
	if err != nil {
		code, dbErr := tokenStateError(err)
		return tokenResponse{}, 0, code, dbErr
	}
	epoch, err := lockTokenAuthority(ctx, tx, initial.AccountID, initial.GrantID, form.Get("client_id"), h.resource)
	if err != nil {
		code, dbErr := tokenStateError(err)
		return tokenResponse{}, 0, code, dbErr
	}
	row, err := q.RotateRefresh(ctx, hash)
	if err != nil {
		code, dbErr := tokenStateError(err)
		return tokenResponse{}, 0, code, dbErr
	}
	if row.AccountID != initial.AccountID || row.GrantID != initial.GrantID || row.Kind != "refresh" || row.ClientID != form.Get("client_id") || row.Resource != h.resource || row.Scope != "mcp:read" || row.AuthEpoch != epoch {
		return tokenResponse{}, 0, "invalid_grant", nil
	}
	// Superseded rows stay useful for reuse detection after token/family expiry.
	if row.Reuse {
		if _, err = q.RevokeGrant(ctx, row.GrantID); err != nil {
			return tokenResponse{}, 0, "", fmt.Errorf("revoke reused refresh grant: %w", err)
		}
		return tokenResponse{}, row.GrantID, "invalid_grant", nil
	}
	now := time.Now()
	if row.RevokedAt != nil || !row.ExpiresAt.After(now) || !row.FamilyExpiresAt.After(now) {
		return tokenResponse{}, 0, "invalid_grant", nil
	}
	response, err := issueTokenPair(ctx, tx, store.CreateTokenFamilyParams{GrantID: row.GrantID, FamilyID: row.FamilyID, RotatedFrom: &hash, Resource: row.Resource, Scope: row.Scope, AuthEpoch: row.AuthEpoch, FamilyExpiresAt: row.FamilyExpiresAt}, true)
	return response, 0, "", err
}
