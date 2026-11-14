package websession

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/store"
	"github.com/jackc/pgx/v5"
)

type fakeStore struct {
	mu                     sync.Mutex
	now                    time.Time
	rows                   map[string]store.GetSessionRow
	account                store.Account
	fail                   string
	reads, touches, writes int
	onGet                  func(context.Context)
	onTouch                func(context.Context)
	onRevoke               func(context.Context)
	touchZero              bool
}

func (s *fakeStore) GetAccount(ctx context.Context, _ int64) (store.Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.check(ctx, "account"); err != nil {
		return store.Account{}, err
	}
	return s.account, nil
}

func (s *fakeStore) check(ctx context.Context, step string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if s.fail == step {
		return errors.New("private database failure")
	}
	return nil
}

func (s *fakeStore) GetSession(ctx context.Context, hash string) (store.GetSessionRow, error) {
	s.mu.Lock()
	s.reads++
	row, ok := s.rows[hash]
	err := s.check(ctx, "get")
	hook := s.onGet
	s.mu.Unlock()
	if hook != nil {
		hook(ctx)
	}
	if err != nil {
		return store.GetSessionRow{}, err
	}
	if !ok {
		return row, pgx.ErrNoRows
	}
	return row, nil
}

func (s *fakeStore) CreateSession(ctx context.Context, p store.CreateSessionParams) (store.CreateSessionRow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.check(ctx, "create"); err != nil {
		return store.CreateSessionRow{}, err
	}
	if len(p.ReplacedHashes) > 0 {
		if err := s.check(ctx, "revoke"); err != nil {
			return store.CreateSessionRow{}, err
		}
	}
	var revoked []string
	for _, hash := range p.ReplacedHashes {
		if old, ok := s.rows[hash]; ok && old.RevokedAt == nil {
			old.RevokedAt = &s.now
			s.rows[hash] = old
			revoked = append(revoked, hash)
		}
	}
	s.writes++
	row := store.GetSessionRow{ID: int64(len(s.rows) + 1), TokenHash: p.TokenHash, AccountID: p.AccountID, CsrfSecret: p.CsrfSecret, SessionAuthEpoch: p.AuthEpoch, AccountAuthEpoch: p.AuthEpoch, CreatedAt: s.now, LastSeenAt: s.now, ExpiresAt: p.ExpiresAt}
	s.rows[p.TokenHash] = row
	return store.CreateSessionRow{ID: row.ID, TokenHash: p.TokenHash, CreatedAt: s.now, LastSeenAt: s.now, ExpiresAt: p.ExpiresAt, RevokedHashes: revoked}, nil
}

func (s *fakeStore) TouchSession(ctx context.Context, hash string) (int64, error) {
	s.mu.Lock()
	hook := s.onTouch
	s.mu.Unlock()
	if hook != nil {
		hook(ctx)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.check(ctx, "touch"); err != nil {
		return 0, err
	}
	row, ok := s.rows[hash]
	if !ok || row.RevokedAt != nil || !row.ExpiresAt.After(s.now) || s.touchZero || s.now.Sub(row.LastSeenAt) < time.Hour {
		return 0, nil
	}
	s.touches++
	row.LastSeenAt = s.now
	row.ExpiresAt = s.now.Add(30 * 24 * time.Hour)
	if capTime := row.CreatedAt.Add(90 * 24 * time.Hour); capTime.Before(row.ExpiresAt) {
		row.ExpiresAt = capTime
	}
	s.rows[hash] = row
	return 1, nil
}

func (s *fakeStore) RevokeSession(ctx context.Context, hash string) (int64, error) {
	s.mu.Lock()
	hook := s.onRevoke
	s.mu.Unlock()
	if hook != nil {
		hook(ctx)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.check(ctx, "revoke"); err != nil {
		return 0, err
	}
	row, ok := s.rows[hash]
	if !ok || row.RevokedAt != nil {
		return 0, nil
	}
	now := s.now
	row.RevokedAt = &now
	s.rows[hash] = row
	return 1, nil
}

func (s *fakeStore) RevokeOwnSession(ctx context.Context, p store.RevokeOwnSessionParams) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.check(ctx, "own"); err != nil {
		return 0, err
	}
	for hash, row := range s.rows {
		if row.ID == p.ID && row.AccountID == p.AccountID && row.RevokedAt == nil {
			now := s.now
			row.RevokedAt = &now
			s.rows[hash] = row
			return 1, nil
		}
	}
	return 0, nil
}

func rawValue(b byte) string {
	v := make([]byte, 32)
	for i := range v {
		v[i] = b
	}
	return base64.RawURLEncoding.EncodeToString(v)
}

func hashValue(v string) string { sum := sha256.Sum256([]byte(v)); return hex.EncodeToString(sum[:]) }

func harness() (*Manager, *fakeStore, string) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	raw := rawValue(1)
	s := &fakeStore{now: now, account: store.Account{ID: 42, AuthEpoch: 3}, rows: map[string]store.GetSessionRow{}}
	s.rows[hashValue(raw)] = store.GetSessionRow{ID: 7, TokenHash: hashValue(raw), AccountID: 42, CsrfSecret: rawValue(2), SessionAuthEpoch: 3, AccountAuthEpoch: 3, CreatedAt: now, LastSeenAt: now, ExpiresAt: now.Add(30 * 24 * time.Hour)}
	m := &Manager{Store: s, Clock: func() time.Time { s.mu.Lock(); defer s.mu.Unlock(); return s.now }, PublicURL: "https://example.invalid/base"}
	return m, s, raw
}

func request(raw string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "https://example.invalid/api/v1/me", nil)
	if raw != "" {
		r.AddCookie(&http.Cookie{Name: "__Host-session", Value: raw})
	}
	return r
}

func serve(m *Manager, r *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	m.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := FromContext(r.Context())
		if !ok || p.AccountID != 42 || p.SessionID == 0 || p.CSRFSecret == "" {
			panic("missing principal")
		}
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(w, r)
	return w
}

func assertStatus(t *testing.T, w *httptest.ResponseRecorder, want int) {
	t.Helper()
	if w.Code != want {
		t.Fatalf("status=%d want=%d body=%s", w.Code, want, w.Body.String())
	}
}

func advance(s *fakeStore, d time.Duration) { s.mu.Lock(); s.now = s.now.Add(d); s.mu.Unlock() }

func editRow(s *fakeStore, raw string, f func(*store.GetSessionRow)) {
	s.mu.Lock()
	row := s.rows[hashValue(raw)]
	f(&row)
	s.rows[hashValue(raw)] = row
	s.mu.Unlock()
}
