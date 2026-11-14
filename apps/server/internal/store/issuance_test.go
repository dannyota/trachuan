package store_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/store"
	"github.com/jackc/pgx/v5"
)

type authorityRow struct {
	value any
	err   error
}

func (r authorityRow) Scan(dst ...any) error {
	if r.err != nil {
		return r.err
	}
	v := reflect.ValueOf(r.value)
	if v.Kind() != reflect.Struct {
		reflect.ValueOf(dst[0]).Elem().Set(v)
		return nil
	}
	if len(dst) != v.NumField() {
		return fmt.Errorf("scan fields=%d want=%d", len(dst), v.NumField())
	}
	for i := range dst {
		reflect.ValueOf(dst[i]).Elem().Set(v.Field(i))
	}
	return nil
}

type authorityTx struct {
	pgx.Tx
	account                       store.Account
	session                       store.GetLiveAuthoritySessionRow
	enrolled, sessionMissing      bool
	locked, committed, rolledBack bool
	writes                        int
	fail                          error
}

func (tx *authorityTx) QueryRow(_ context.Context, sql string, _ ...any) pgx.Row {
	switch {
	case strings.Contains(sql, "LockCredentialAccount"):
		tx.locked = true
		return authorityRow{value: tx.account}
	case strings.Contains(sql, "GetLiveAuthoritySession"):
		if !tx.locked {
			return authorityRow{err: errors.New("unlocked authority read")}
		}
		if tx.sessionMissing {
			return authorityRow{err: pgx.ErrNoRows}
		}
		return authorityRow{value: tx.session}
	case strings.Contains(sql, "HasTOTP"):
		if !tx.locked {
			return authorityRow{err: errors.New("unlocked factor read")}
		}
		return authorityRow{value: tx.enrolled}
	case strings.Contains(sql, "CreatePersonalToken"):
		tx.writes++
		return authorityRow{value: store.PersonalToken{}, err: tx.fail}
	default:
		return authorityRow{err: fmt.Errorf("unexpected query")}
	}
}
func (tx *authorityTx) Commit(context.Context) error   { tx.committed = true; return nil }
func (tx *authorityTx) Rollback(context.Context) error { tx.rolledBack = true; return nil }

type authorityDB struct {
	store.DBTX
	tx *authorityTx
}

func (db authorityDB) BeginTx(_ context.Context, opts pgx.TxOptions) (pgx.Tx, error) {
	if opts.IsoLevel != pgx.ReadCommitted {
		return nil, errors.New("isolation must refresh snapshots after the account lock")
	}
	return db.tx, nil
}

func TestWithAccount_RefusesStaleOrBlockedAuthority(t *testing.T) {
	now := time.Now()
	for _, tc := range []struct {
		name    string
		account store.Account
		want    bool
	}{
		{"current", store.Account{ID: 1, AuthEpoch: 7}, true},
		{"stale", store.Account{ID: 1, AuthEpoch: 8}, false},
		{"blocked", store.Account{ID: 1, AuthEpoch: 7, BlockedAt: &now}, false},
		{"deleting", store.Account{ID: 1, AuthEpoch: 7, DeleteRequestedAt: &now}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tx := &authorityTx{account: tc.account}
			wrote := false
			err := store.WithAccount(context.Background(), authorityDB{tx: tx}, 1, 7, func(*store.Queries) error { wrote = true; return nil })
			if tc.want {
				if err != nil || !wrote || !tx.committed {
					t.Fatal("current authority refused")
				}
			} else if !errors.Is(err, store.ErrUnauthorized) || wrote || tx.committed {
				t.Fatal("invalid authority issued")
			}
			if !tx.locked || !tx.rolledBack {
				t.Fatal("transaction or lock missing")
			}
		})
	}
}

func TestWebToken_RecentProofAndFactor(t *testing.T) {
	now := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	boundary := now.Add(-10 * time.Minute)
	stale := boundary.Add(-time.Nanosecond)
	for _, tc := range []struct {
		name          string
		proof, factor *time.Time
		enrolled      bool
		want          error
	}{
		{"legacy", nil, nil, false, store.ErrReauthRequired},
		{"stale proof", &stale, nil, false, store.ErrReauthRequired},
		{"boundary", &boundary, nil, false, nil},
		{"recent", &now, nil, false, nil},
		{"factor missing", &now, nil, true, store.ErrFactorRequired},
		{"factor stale", &now, &stale, true, store.ErrFactorRequired},
		{"factor boundary", &now, &boundary, true, nil},
		{"proof first", nil, &now, true, store.ErrReauthRequired},
		{"both recent", &now, &now, true, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tx := &authorityTx{account: store.Account{ID: 1, AuthEpoch: 7}, session: store.GetLiveAuthoritySessionRow{ProofAt: tc.proof, FactorAt: tc.factor, CheckedAt: now}, enrolled: tc.enrolled}
			q := store.New(authorityDB{tx: tx})
			_, err := q.CreateWebPersonalToken(context.Background(), store.WebPersonalTokenParams{Token: store.CreatePersonalTokenParams{AccountID: 1, AuthEpoch: 7}, SessionID: 2})
			if !errors.Is(err, tc.want) {
				t.Fatalf("error=%v want=%v", err, tc.want)
			}
			if (tx.writes == 1) != (tc.want == nil) || tx.committed != (tc.want == nil) {
				t.Fatal("incorrect issuance or commit")
			}
		})
	}
}

func TestWebToken_RejectsRevokedSessionAndRollsBackLimit(t *testing.T) {
	now := time.Now()
	for _, missing := range []bool{true, false} {
		tx := &authorityTx{account: store.Account{ID: 1}, session: store.GetLiveAuthoritySessionRow{ProofAt: &now, CheckedAt: now}, sessionMissing: missing, fail: pgx.ErrNoRows}
		_, err := store.New(authorityDB{tx: tx}).CreateWebPersonalToken(context.Background(), store.WebPersonalTokenParams{Token: store.CreatePersonalTokenParams{AccountID: 1}})
		want := store.ErrTokenLimit
		if missing {
			want = store.ErrUnauthorized
		}
		if !errors.Is(err, want) || tx.committed {
			t.Fatal("invalid request committed")
		}
	}
}
