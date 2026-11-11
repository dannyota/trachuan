package websession

import (
	"context"
	"fmt"
	"net/http"

	"danny.vn/trachuan/apps/server/internal/apierror"
	"danny.vn/trachuan/apps/server/internal/store"
)

// RevokeSession revokes a raw session token and evicts its cached lookup.
func (m *Manager) RevokeSession(ctx context.Context, raw string) error {
	if !validValue(raw) {
		return nil
	}
	hash := hashToken(raw)
	rev := revocation{hash: hash}
	m.fence(rev)
	defer m.unfence(rev)
	if m.Store == nil {
		return errUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, authQueryDeadline)
	defer cancel()
	if _, err := m.Store.RevokeSession(ctx, hash); err != nil || ctx.Err() != nil {
		return fmt.Errorf("revoke session: %w", errUnavailable)
	}
	return nil
}

// RevokeOwnSession revokes only the caller's session and returns the row count.
// A zero count means the id is absent, already revoked or belongs to another account.
func (m *Manager) RevokeOwnSession(ctx context.Context, accountID, sessionID int64) (int64, error) {
	rev := revocation{accountID: accountID, sessionID: sessionID}
	m.fence(rev)
	defer m.unfence(rev)
	if m.Store == nil {
		return 0, errUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, authQueryDeadline)
	defer cancel()
	count, err := m.Store.RevokeOwnSession(ctx, store.RevokeOwnSessionParams{AccountID: accountID, ID: sessionID})
	if err != nil || ctx.Err() != nil {
		return 0, fmt.Errorf("revoke own session: %w", errUnavailable)
	}
	return count, nil
}

// Logout revokes the presented session and clears browser cookies. Route it as
// POST /api/v1/auth/logout behind Middleware and CSRF.
func (m *Manager) Logout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	raw, ok := sessionToken(r)
	if !ok {
		apierror.Write(w, 401, "unauthenticated")
		return
	}
	if err := m.RevokeSession(r.Context(), raw); err != nil {
		apierror.Write(w, 503, "unavailable")
		return
	}
	writeCookie(w, "", -1)
	w.Header().Set("Clear-Site-Data", `"cookies"`)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}

type revocation struct {
	hash                 string
	accountID, sessionID int64
}

func (rev revocation) matches(row store.GetSessionRow) bool {
	if rev.hash != "" {
		return rev.hash == row.TokenHash
	}
	return rev.accountID == row.AccountID && rev.sessionID == row.ID
}

type lookupFence struct {
	generation uint64
	hash       string
	revoked    map[revocation]struct{}
}

func (f *lookupFence) mark(rev revocation) {
	if rev.hash == "" || rev.hash == f.hash {
		f.revoked[rev] = struct{}{}
	}
}

func (m *Manager) fence(rev revocation) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.revoking == nil {
		m.revoking = make(map[revocation]int)
	}
	m.revoking[rev]++
	for flight := range m.flights {
		flight.mark(rev)
	}
	m.evict(rev)
}

func (m *Manager) unfence(rev revocation) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.revoking[rev]--
	if m.revoking[rev] == 0 {
		delete(m.revoking, rev)
	}
	m.evict(rev)
}

func (m *Manager) evict(rev revocation) {
	for hash, entry := range m.entries {
		if rev.matches(entry.row) {
			delete(m.entries, hash)
		}
	}
}

// EvictAccount removes every cached session for the account and fences old lookups.
func (m *Manager) EvictAccount(accountID int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for hash, entry := range m.entries {
		if entry.row.AccountID == accountID {
			delete(m.entries, hash)
		}
	}
	if len(m.flights) > 0 {
		m.generation++
		if m.accountGenerations == nil {
			m.accountGenerations = make(map[int64]uint64)
		}
		m.accountGenerations[accountID] = m.generation
	}
}
