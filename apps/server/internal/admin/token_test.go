package admin

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/credential"
	"danny.vn/trachuan/apps/server/internal/store"
)

type fakeStore struct {
	account       store.Account
	err           error
	createErr     error
	accountParams store.CreateAccountParams
	tokenParams   store.CreatePersonalTokenParams
	revoked       string
	blocked       int64
	reads, writes int
}

func (s *fakeStore) CreateAccount(_ context.Context, p store.CreateAccountParams) (store.Account, error) {
	s.accountParams = p
	return s.account, s.err
}

func (s *fakeStore) GetAccount(_ context.Context, id int64) (store.Account, error) {
	s.reads++
	if id != s.account.ID {
		return store.Account{}, errors.New("missing account")
	}
	return s.account, s.err
}

func (s *fakeStore) BlockAccount(_ context.Context, id int64) (store.BlockAccountRow, error) {
	s.blocked = id
	return store.BlockAccountRow{}, s.err
}

func (s *fakeStore) CreatePersonalToken(_ context.Context, p store.CreatePersonalTokenParams) (store.PersonalToken, error) {
	s.writes++
	s.tokenParams = p
	return store.PersonalToken{}, s.createErr
}

func (s *fakeStore) RevokePersonalToken(_ context.Context, hash string) error {
	s.revoked = hash
	return s.err
}

func service() (*Service, *fakeStore, time.Time) {
	now := time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC)
	s := &fakeStore{account: store.Account{ID: 7, AuthEpoch: 3}}
	return &Service{Store: s, Clock: func() time.Time { return now }, Rand: bytes.NewReader(make([]byte, 32))}, s, now
}

func TestAdmin_TokenLifetimeBounds(t *testing.T) {
	for _, tc := range []struct {
		name           string
		lifetime, want time.Duration
		invalid        bool
	}{
		{"default", 0, 90 * 24 * time.Hour, false},
		{"short", time.Hour, time.Hour, false},
		{"maximum", 365 * 24 * time.Hour, 365 * 24 * time.Hour, false},
		{"negative", -time.Second, 0, true},
		{"above maximum", 365*24*time.Hour + time.Nanosecond, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, s, now := service()
			plain, err := a.CreateToken(context.Background(), 7, "cli", tc.lifetime)
			if tc.invalid {
				if err == nil || plain != "" || s.reads != 0 || s.writes != 0 {
					t.Fatal("invalid lifetime reached store or returned token")
				}
				return
			}
			if err != nil || plain == "" {
				t.Fatalf("create token: %v", err)
			}
			_, hash, _ := credential.Issue(bytes.NewReader(make([]byte, 32)))
			p := s.tokenParams
			if p.TokenHash != hash || p.AccountID != 7 || p.Name != "cli" || p.AuthEpoch != 3 || !p.ExpiresAt.Equal(now.Add(tc.want)) {
				t.Fatal("incorrect stored token metadata")
			}
		})
	}
}

func TestAdmin_CreateTokenFailuresReturnNoToken(t *testing.T) {
	for _, name := range []string{"blocked", "unknown", "database", "entropy", "insert"} {
		t.Run(name, func(t *testing.T) {
			a, s, now := service()
			id := int64(7)
			switch name {
			case "blocked":
				s.account.BlockedAt = &now
			case "unknown":
				id = 8
			case "database":
				s.err = errors.New("sensitive query")
			case "entropy":
				a.Rand = bytes.NewReader(nil)
			case "insert":
				s.createErr = errors.New("sensitive query")
			}
			plain, err := a.CreateToken(context.Background(), id, "cli", 0)
			if plain != "" || err == nil || strings.Contains(err.Error(), "sensitive") {
				t.Fatal("failed issuance disclosed a credential or database details")
			}
			if name != "insert" && s.writes != 0 {
				t.Fatal("failed validation wrote a token")
			}
		})
	}
}

func TestAdmin_RevokeTokenUsesHash(t *testing.T) {
	a, s, _ := service()
	plain, hash, _ := credential.Issue(bytes.NewReader(make([]byte, 32)))
	if err := a.RevokeToken(context.Background(), hash); err != nil || s.revoked != hash {
		t.Fatalf("revoke: %v", err)
	}
	for _, invalid := range []string{plain, "", strings.Repeat("A", 64), strings.Repeat("a", 63), strings.Repeat("g", 64)} {
		s.revoked = ""
		if err := a.RevokeToken(context.Background(), invalid); err == nil || s.revoked != "" {
			t.Fatal("invalid hash reached store")
		}
	}
}
