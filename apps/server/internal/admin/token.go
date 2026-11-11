package admin

import (
	"context"
	"crypto/rand"
	"fmt"
	"time"

	"danny.vn/trachuan/apps/server/internal/credential"
	"danny.vn/trachuan/apps/server/internal/store"
)

const (
	defaultLifetime         = 90 * 24 * time.Hour
	maxLifetime             = 365 * 24 * time.Hour
	errLifetime     failure = "personal token lifetime must be positive and at most 365 days"
	errBlocked      failure = "account is blocked"
	errHash         failure = "invalid personal token identifier"
)

// CreateToken returns a token once, after persisting only its digest and issue
// epoch. A zero lifetime means 90 days; lifetimes above 365 days are refused.
func (s *Service) CreateToken(ctx context.Context, accountID int64, name string, lifetime time.Duration) (plain string, err error) {
	if lifetime == 0 {
		lifetime = defaultLifetime
	}
	if lifetime < 0 || lifetime > maxLifetime {
		return "", fmt.Errorf("create personal token: %w", errLifetime)
	}
	account, err := s.Store.GetAccount(ctx, accountID)
	if err != nil {
		return "", fmt.Errorf("read account: %w", errStore)
	}
	if account.BlockedAt != nil {
		return "", fmt.Errorf("create personal token: %w", errBlocked)
	}
	random := s.Rand
	if random == nil {
		random = rand.Reader
	}
	plain, hash, err := credential.Issue(random)
	if err != nil {
		return "", fmt.Errorf("create personal token: %w", err)
	}
	now := time.Now()
	if s.Clock != nil {
		now = s.Clock()
	}
	_, err = s.Store.CreatePersonalToken(ctx, store.CreatePersonalTokenParams{
		TokenHash: hash, AccountID: accountID, Name: name,
		ExpiresAt: now.Add(lifetime), AuthEpoch: account.AuthEpoch,
	})
	if err != nil {
		return "", fmt.Errorf("save personal token: %w", errStore)
	}
	return plain, nil
}

// RevokeToken revokes the lowercase SHA-256 hex identifier stored for a token.
// Other processes observe revocation within their credential cache window.
func (s *Service) RevokeToken(ctx context.Context, hash string) error {
	if len(hash) != 64 {
		return fmt.Errorf("revoke personal token: %w", errHash)
	}
	for _, c := range []byte(hash) {
		switch {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'f':
		default:
			return fmt.Errorf("revoke personal token: %w", errHash)
		}
	}
	if err := s.Store.RevokePersonalToken(ctx, hash); err != nil {
		return fmt.Errorf("revoke personal token: %w", errStore)
	}
	return nil
}
