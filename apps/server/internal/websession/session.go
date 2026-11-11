// Package websession authenticates web cookies and protects web mutations.
package websession

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"danny.vn/trachuan/apps/server/internal/store"
)

const (
	idleLifetime      = 30 * 24 * time.Hour
	absoluteLifetime  = 90 * 24 * time.Hour
	maxCacheTTL       = 10 * time.Second
	authQueryDeadline = 2 * time.Second
	maxEntries        = 10000
)

type failure string

func (f failure) Error() string { return string(f) }

const (
	errUnauthenticated failure = "unauthenticated"
	errUnavailable     failure = "session service unavailable"
)

// Principal identifies the account and session authenticated by Middleware.
type Principal struct {
	AccountID, SessionID int64
	CSRFSecret           string
}

// Store supplies session state and account eligibility. Generated Queries satisfy it.
type Store interface {
	GetAccount(context.Context, int64) (store.Account, error)
	CreateSession(context.Context, store.CreateSessionParams) (store.CreateSessionRow, error)
	GetSession(context.Context, string) (store.GetSessionRow, error)
	TouchSession(context.Context, string) (int64, error)
	RevokeSession(context.Context, string) (int64, error)
	RevokeOwnSession(context.Context, store.RevokeOwnSessionParams) (int64, error)
}

// Manager issues and authenticates sessions. Configure before use, share one
// instance across requests, and do not copy it after use. A negative CacheTTL
// disables reuse; zero or values above ten seconds use ten seconds.
type Manager struct {
	Store     Store
	Clock     func() time.Time
	Rand      io.Reader
	PublicURL string
	CacheTTL  time.Duration

	randomMu           sync.Mutex
	mu                 sync.Mutex
	entries            map[string]cacheEntry
	flights            map[*lookupFence]struct{}
	revoking           map[revocation]int
	generation         uint64
	accountGenerations map[int64]uint64
}

// CreateSession revokes presented sessions before creating a fresh login session.
// It publishes the cookie only after all database operations succeed.
func (m *Manager) CreateSession(ctx context.Context, w http.ResponseWriter, r *http.Request, accountID int64) error {
	ctx, cancel := context.WithTimeout(ctx, authQueryDeadline)
	defer cancel()
	if m.Store == nil {
		return errUnavailable
	}
	for _, cookie := range r.CookiesNamed(sessionCookieName) {
		if validValue(cookie.Value) {
			if err := m.RevokeSession(ctx, cookie.Value); err != nil {
				return fmt.Errorf("replace session: %w", err)
			}
		}
	}
	account, err := m.Store.GetAccount(ctx, accountID)
	if ctx.Err() != nil || err != nil {
		return fmt.Errorf("read session account: %w", errUnavailable)
	}
	if account.BlockedAt != nil || account.DeleteRequestedAt != nil {
		return errUnauthenticated
	}
	token, secret, err := m.newValues()
	if err != nil {
		return fmt.Errorf("create session values: %w", errUnavailable)
	}
	row, err := m.Store.CreateSession(ctx, store.CreateSessionParams{TokenHash: hashToken(token), AccountID: accountID, CsrfSecret: secret, AuthEpoch: account.AuthEpoch, ExpiresAt: m.now().Add(idleLifetime)})
	if ctx.Err() != nil || err != nil {
		m.EvictAccount(accountID)
		return fmt.Errorf("create session: %w", errUnavailable)
	}
	// Preserve marks for lookups that read a session before SQL revoked it.
	for _, digest := range row.RevokedHashes {
		rev := revocation{hash: digest}
		m.fence(rev)
		m.unfence(rev)
	}
	expires := minTime(row.ExpiresAt, row.CreatedAt.Add(absoluteLifetime))
	if !expires.After(m.now()) {
		return errUnavailable
	}
	setSessionCookie(w, token, expires, m.now())
	return nil
}

func (m *Manager) newValues() (string, string, error) {
	m.randomMu.Lock()
	defer m.randomMu.Unlock()
	reader := m.Rand
	if reader == nil {
		reader = rand.Reader
	}
	values := make([]byte, 64)
	if _, err := io.ReadFull(reader, values); err != nil {
		return "", "", fmt.Errorf("read randomness: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(values[:32]), base64.RawURLEncoding.EncodeToString(values[32:]), nil
}

func (m *Manager) now() time.Time {
	if m.Clock != nil {
		return m.Clock()
	}
	return time.Now()
}

func (m *Manager) ttl() time.Duration {
	if m.CacheTTL == 0 || m.CacheTTL > maxCacheTTL {
		return maxCacheTTL
	}
	return m.CacheTTL
}

func validValue(raw string) bool {
	if len(raw) != 43 {
		return false
	}
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(raw)
	return err == nil && len(decoded) == 32
}

func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
