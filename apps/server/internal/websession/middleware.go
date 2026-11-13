package websession

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"danny.vn/trachuan/apps/server/internal/apierror"
	"danny.vn/trachuan/apps/server/internal/store"
	"github.com/jackc/pgx/v5"
)

type principalKey struct{}

func FromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(Principal)
	return p, ok
}

func (m *Manager) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, ok := sessionToken(r)
		if !ok {
			if r.Method == http.MethodGet && r.URL.Path == "/api/v1/me" && !hasSessionCookie(r) {
				w.Header().Set("Cache-Control", "no-store")
				w.WriteHeader(http.StatusNoContent)
				return
			}
			apierror.Write(w, 401, "unauthenticated")
			return
		}
		row, renew, err := m.lookup(r.Context(), raw)
		if err != nil {
			if errors.Is(err, errUnavailable) {
				apierror.Write(w, 503, "unavailable")
			} else {
				apierror.Write(w, 401, "unauthenticated")
			}
			return
		}
		if renew {
			setSessionCookie(w, raw, minTime(row.ExpiresAt, row.CreatedAt.Add(absoluteLifetime)), m.now())
		}
		p := Principal{AccountID: row.AccountID, SessionID: row.ID, CSRFSecret: row.CsrfSecret}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey{}, p)))
	})
}

func hasSessionCookie(r *http.Request) bool {
	for _, header := range r.Header.Values("Cookie") {
		for _, part := range strings.Split(header, ";") {
			name, _, _ := strings.Cut(strings.TrimSpace(part), "=")
			if strings.TrimSpace(name) == sessionCookieName {
				return true
			}
		}
	}
	return false
}

type cacheEntry struct {
	row   store.GetSessionRow
	until time.Time
}

func (m *Manager) lookup(ctx context.Context, raw string) (store.GetSessionRow, bool, error) {
	hash := hashToken(raw)
	started := m.now()
	m.mu.Lock()
	now := m.now()
	if entry, ok := m.entries[hash]; ok {
		if entry.until.After(now) && validSession(entry.row, now) && now.Sub(entry.row.LastSeenAt) < time.Hour {
			m.mu.Unlock()
			return entry.row, false, nil
		}
		delete(m.entries, hash)
	}
	fence := &lookupFence{hash: hash, generation: m.generation, revoked: make(map[revocation]struct{})}
	for rev := range m.revoking {
		fence.mark(rev)
	}
	if m.flights == nil {
		m.flights = make(map[*lookupFence]struct{})
	}
	m.flights[fence] = struct{}{}
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		delete(m.flights, fence)
		oldest := m.generation
		for f := range m.flights {
			oldest = min(oldest, f.generation)
		}
		for id, generation := range m.accountGenerations {
			if generation <= oldest {
				delete(m.accountGenerations, id)
			}
		}
		m.mu.Unlock()
	}()
	if m.Store == nil {
		return store.GetSessionRow{}, false, errUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, authQueryDeadline)
	defer cancel()
	row, err := m.Store.GetSession(ctx, hash)
	if ctx.Err() != nil {
		return row, false, errUnavailable
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return row, false, errUnauthenticated
	}
	if err != nil {
		return row, false, fmt.Errorf("lookup session: %w", errUnavailable)
	}
	if !validSession(row, m.now()) {
		return row, false, errUnauthenticated
	}
	renew := false
	if m.now().Sub(row.LastSeenAt) >= time.Hour {
		count, touchErr := m.Store.TouchSession(ctx, hash)
		if touchErr != nil || ctx.Err() != nil {
			return row, false, fmt.Errorf("touch session: %w", errUnavailable)
		}
		if count > 0 {
			row, err = m.Store.GetSession(ctx, hash)
			if ctx.Err() != nil {
				return row, false, errUnavailable
			}
			if errors.Is(err, pgx.ErrNoRows) {
				return row, false, errUnauthenticated
			}
			if err != nil {
				return row, false, fmt.Errorf("read touched session: %w", errUnavailable)
			}
			renew = true
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	now = m.now()
	if ctx.Err() != nil {
		return row, false, errUnavailable
	}
	if !validSession(row, now) {
		return row, false, errUnauthenticated
	}
	freshness := m.ttl()
	if freshness < 0 {
		freshness = maxCacheTTL
	}
	if now.Sub(started) >= freshness {
		return row, false, errUnavailable
	}
	until := minTime(started.Add(m.ttl()), minTime(row.ExpiresAt, row.CreatedAt.Add(absoluteLifetime)))
	if m.accountGenerations[row.AccountID] > fence.generation {
		return row, false, errUnauthenticated
	}
	for rev := range fence.revoked {
		if rev.matches(row) {
			return row, false, errUnauthenticated
		}
	}
	if until.After(now) {
		if m.entries == nil {
			m.entries = make(map[string]cacheEntry)
		}
		if entry, ok := m.entries[hash]; ok {
			until = minTime(until, entry.until)
		} else if len(m.entries) >= maxEntries {
			for key := range m.entries {
				delete(m.entries, key)
				break
			}
		}
		m.entries[hash] = cacheEntry{row: row, until: until}
	}
	return row, renew, nil
}

func validSession(row store.GetSessionRow, now time.Time) bool {
	return row.RevokedAt == nil && row.BlockedAt == nil && row.DeleteRequestedAt == nil && row.SessionAuthEpoch == row.AccountAuthEpoch && row.ExpiresAt.After(now) && row.CreatedAt.Add(absoluteLifetime).After(now)
}
