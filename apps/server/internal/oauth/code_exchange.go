package oauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"net/url"
	"time"

	"danny.vn/trachuan/apps/server/internal/store"
	"github.com/jackc/pgx/v5"
)

func (h *tokenHandler) exchangeCode(ctx context.Context, tx pgx.Tx, form url.Values) (tokenResponse, int64, string, error) {
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(form.Get("code"))))
	identity, err := store.New(tx).GetCodeIdentity(ctx, hash)
	if err != nil {
		code, dbErr := tokenStateError(err)
		return tokenResponse{}, 0, code, dbErr
	}
	accountID, grantID := identity.AccountID, identity.GrantID
	epoch, client, err := lockTokenAuthority(ctx, tx, accountID, grantID, form.Get("client_id"), h.resource, "authorization_code")
	if err != nil {
		code, dbErr := tokenStateError(err)
		return tokenResponse{}, 0, code, dbErr
	}
	row, err := store.New(tx).ConsumeCode(ctx, hash)
	if err != nil {
		code, dbErr := tokenStateError(err)
		return tokenResponse{}, 0, code, dbErr
	}
	redirect, hasRedirect := form["redirect_uri"]
	if row.AccountID != accountID || row.GrantID != grantID || row.ClientID != form.Get("client_id") || row.Resource != h.resource || row.Scope != "mcp:read" || row.AuthEpoch != epoch || row.RevokedAt != nil ||
		(hasRedirect && redirect[0] != row.RedirectUri) || !VerifyS256(row.CodeChallenge, form.Get("code_verifier")) {
		return tokenResponse{}, 0, "invalid_grant", nil
	}
	if row.Replay {
		if _, err = store.New(tx).RevokeGrant(ctx, grantID); err != nil {
			return tokenResponse{}, 0, "", fmt.Errorf("revoke replayed code grant: %w", err)
		}
		return tokenResponse{}, grantID, "invalid_grant", nil
	}
	if !row.ExpiresAt.After(time.Now()) {
		return tokenResponse{}, 0, "invalid_grant", nil
	}
	pruned, err := store.New(tx).PruneTokenFamilies(ctx, grantID)
	if err != nil {
		return tokenResponse{}, 0, "", fmt.Errorf("cap token families: %w", err)
	}
	_, familyHash, err := NewSecret(rand.Reader, "")
	if err != nil {
		return tokenResponse{}, 0, "", fmt.Errorf("create token family: %w", err)
	}
	response, err := issueTokenPair(ctx, tx, store.CreateTokenFamilyParams{GrantID: grantID, FamilyID: fmt.Sprintf("%x", familyHash), Resource: row.Resource, Scope: row.Scope, AuthEpoch: row.AuthEpoch, FamilyExpiresAt: time.Now().Add(30 * 24 * time.Hour)}, client.Allows("refresh_token"))
	if pruned > 0 {
		return response, grantID, "", err
	}
	return response, 0, "", err
}
