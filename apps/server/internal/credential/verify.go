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
	maxCacheTTL       = 10 * time.Second
	authQueryDeadline = 2 * time.Second

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
	// QueryTimeout can shorten the two-second auth database deadline.
	QueryTimeout time.Duration

	mu                 sync.Mutex
	entries            map[string]*list.Element
	oldest             list.List
	touches            map[string]time.Time
	used               list.List
	flights            map[*lookupFlight]struct{}
	generation         uint64
	accountGenerations map[int64]uint64
}

// Verify authenticates a raw personal token without the Bearer scheme.
func (v *Verifier) Verify(ctx context.Context, bearer string) (Principal, error) {
	if !validToken(bearer) {
		return Principal{}, ErrUnauthorized
	}
	if v.Store == nil || v.Audience == "" {
		return Principal{}, ErrUnavailable
	}
	hash := tokenHash(bearer)
	entry, flight, ok := v.startLookup(hash, v.now())
	if ok {
		// Cached hits skip availability checks and touches because the lookup is
		// cached by design: docs/design/auth.md, "Token checks and revocation".
		if !entry.expires.After(v.now()) {
			return Principal{}, ErrUnauthorized
		}
		return entry.principal, nil
	}
	defer v.finishLookup(flight)
	timeout := v.QueryTimeout
	if timeout <= 0 || timeout > authQueryDeadline {
		timeout = authQueryDeadline
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	started := v.now()
	row, err := v.Store.GetCredential(ctx, hash)
	if ctx.Err() != nil {
		return Principal{}, fmt.Errorf("verify credential: %w", ErrUnavailable)
	}
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
	if v.admitTouch(hash, v.now()) {
		if err := v.Store.TouchPersonalToken(ctx, hash); err != nil {
			return Principal{}, fmt.Errorf("touch credential: %w", ErrUnavailable)
		}
	}
	if ctx.Err() != nil {
		return Principal{}, fmt.Errorf("verify credential: %w", ErrUnavailable)
	}
	now := v.now()
	if !row.ExpiresAt.After(now) {
		return Principal{}, ErrUnauthorized
	}
	ttl := v.ttl()
	freshness := ttl
	if freshness < 0 {
		// Disabling cached reuse still bounds the age of a fresh lookup.
		freshness = maxCacheTTL
	}
	if now.Sub(started) > freshness {
		return Principal{}, fmt.Errorf("verify credential freshness: %w", ErrUnavailable)
	}
	entry = cacheEntry{
		hash:      hash,
		principal: Principal{AccountID: row.AccountID, Scope: "mcp:read", Audience: v.Audience},
		expires:   row.ExpiresAt,
		until:     minTime(started.Add(ttl), row.ExpiresAt),
	}
	if !v.remember(entry, now, flight) {
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
	if v.CacheTTL == 0 || v.CacheTTL > maxCacheTTL {
		return maxCacheTTL
	}
	return v.CacheTTL
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
