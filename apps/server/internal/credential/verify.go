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

	ErrUnauthorized              = store.ErrUnauthorized
	ErrInsufficientScope failure = "insufficient scope"
	ErrUnavailable       failure = "credential service unavailable"
)

type Principal struct {
	AccountID int64
	Scope     string
	Audience  string
}

type Store interface {
	GetCredential(context.Context, string) (store.GetCredentialRow, error)
	TouchPersonalToken(context.Context, string) error
}

type Verifier struct {
	Store        Store
	Clock        func() time.Time
	CacheTTL     time.Duration
	Audience     string
	QueryTimeout time.Duration

	mu                 sync.Mutex
	entries            map[string]*list.Element
	oldest             list.List
	touches            map[string]time.Time
	used               list.List
	flights            map[*lookupFlight]struct{}
	generation         uint64
	accountGenerations map[int64]uint64
	grantGenerations   map[int64]uint64
}

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
	row, grantID, scope, touch, err := v.lookup(ctx, bearer, hash)
	if ctx.Err() != nil {
		return Principal{}, fmt.Errorf("verify credential: %w", ErrUnavailable)
	}
	if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, ErrUnauthorized) {
		return Principal{}, ErrUnauthorized
	}
	if err != nil {
		return Principal{}, fmt.Errorf("verify credential: %w", ErrUnavailable)
	}
	if row.RevokedAt != nil || row.BlockedAt != nil || row.AccountAuthEpoch != row.TokenAuthEpoch || !row.ExpiresAt.After(v.now()) {
		return Principal{}, ErrUnauthorized
	}
	if scope != "mcp:read" {
		return Principal{}, ErrInsufficientScope
	}
	if v.admitTouch(hash, v.now()) {
		if err := touch(ctx); err != nil {
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
		freshness = maxCacheTTL
	}
	if now.Sub(started) > freshness {
		return Principal{}, fmt.Errorf("verify credential freshness: %w", ErrUnavailable)
	}
	entry = cacheEntry{
		hash:      hash,
		grantID:   grantID,
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
