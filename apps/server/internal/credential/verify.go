package credential

import (
	"container/list"
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"danny.vn/trachuan/apps/server/internal/store"
	"github.com/jackc/pgx/v5"
)

const (
	// ErrUnauthorized means the credential is missing, invalid or no longer valid.
	ErrUnauthorized failure = "unauthorized"
	// ErrUnavailable means the verifier cannot check the credential.
	ErrUnavailable failure = "credential service unavailable"
)

// Principal identifies an authenticated account and its MCP permissions.
type Principal struct {
	AccountID int64
	Scope     string
	Audience  string
}

// Store supplies credential state and records successful token use.
type Store interface {
	GetCredential(context.Context, string) (store.GetCredentialRow, error)
	TouchPersonalToken(context.Context, string) error
}

// Verifier checks personal tokens. Configure its fields before use, do not copy
// it after use, and share one instance across requests.
type Verifier struct {
	Store    Store
	Clock    func() time.Time
	CacheTTL time.Duration
	Audience string

	mu      sync.Mutex
	entries map[string]*list.Element
	oldest  list.List
	touches map[string]time.Time
	used    list.List
}

// Verify authenticates a raw personal token without the Bearer scheme.
func (v *Verifier) Verify(ctx context.Context, bearer string) (Principal, error) {
	if !validToken(bearer) {
		return Principal{}, ErrUnauthorized
	}
	if v.Store == nil || v.Audience == "" {
		return Principal{}, ErrUnavailable
	}
	started := v.now()
	hash := tokenHash(bearer)
	entry, ok := v.cached(hash, started)
	if !ok {
		row, err := v.Store.GetCredential(ctx, hash)
		if errors.Is(err, pgx.ErrNoRows) {
			return Principal{}, ErrUnauthorized
		}
		if err != nil {
			// Store errors can contain SQL, parameters or connection details.
			return Principal{}, fmt.Errorf("verify credential: %w", ErrUnavailable)
		}
		if row.RevokedAt != nil || row.BlockedAt != nil || row.AccountAuthEpoch != row.TokenAuthEpoch || !row.ExpiresAt.After(v.now()) {
			return Principal{}, ErrUnauthorized
		}
		entry = cacheEntry{
			hash:      hash,
			principal: Principal{AccountID: row.AccountID, Scope: "mcp:read", Audience: v.Audience},
			expires:   row.ExpiresAt,
			until:     minTime(started.Add(v.ttl()), row.ExpiresAt),
		}
		v.remember(entry, v.now())
	}
	if v.admitTouch(hash, v.now()) {
		// Usage tracking must never change the authorization result.
		_ = v.Store.TouchPersonalToken(ctx, hash)
	}
	if !entry.expires.After(v.now()) {
		return Principal{}, ErrUnauthorized
	}
	return entry.principal, nil
}

func (v *Verifier) now() time.Time {
	if v.Clock == nil {
		return time.Now()
	}
	return v.Clock()
}

func (v *Verifier) ttl() time.Duration {
	if v.CacheTTL == 0 || v.CacheTTL > 10*time.Second {
		return 10 * time.Second
	}
	return v.CacheTTL
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
