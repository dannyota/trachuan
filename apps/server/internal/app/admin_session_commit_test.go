package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/store"
	"github.com/jackc/pgx/v5"
)

type adminSessionCreatedRow struct{ row store.CreateSessionRow }

func (r adminSessionCreatedRow) Scan(dst ...any) error {
	value := reflect.ValueOf(r.row)
	if len(dst) != value.NumField() {
		return errors.New("unexpected session fields")
	}
	for i := range dst {
		reflect.ValueOf(dst[i]).Elem().Set(value.Field(i))
	}
	return nil
}

type adminSessionCommitFixture struct {
	sessionAccountFixture
	store.DBTX
	path, hash                     string
	live, commitError, revokeError bool
	cancel                         context.CancelFunc
}

func newAdminSessionCommitFixture(path string) *adminSessionCommitFixture {
	sub := "loadcheck:commit-unit"
	return &adminSessionCommitFixture{sessionAccountFixture: sessionAccountFixture{account: store.Account{ID: 42, AuthEpoch: 7, GoogleSub: &sub}}, path: path}
}

func (s *adminSessionCommitFixture) QueryRow(_ context.Context, query string, args ...any) pgx.Row {
	if !strings.HasPrefix(query, "-- name: CreateSession") {
		panic("unexpected session query")
	}
	s.hash = args[2].(string)
	now := time.Now()
	return adminSessionCreatedRow{row: store.CreateSessionRow{ID: 1, AccountID: args[1].(int64), TokenHash: s.hash, CsrfSecret: args[3].(string), AuthEpoch: args[4].(int32), ExpiresAt: args[5].(time.Time), CreatedAt: now, LastSeenAt: now, ProofAt: &now}}
}

func (s *adminSessionCommitFixture) withAdminSession(_ context.Context, _ int64, _ int32, write func(*store.Queries) error) error {
	if err := write(store.New(s)); err != nil {
		return err
	}
	s.live = true
	if s.cancel != nil {
		s.cancel()
	}
	if s.commitError {
		return errors.New("synthetic error after commit")
	}
	return nil
}

func (s *adminSessionCommitFixture) RevokeSession(ctx context.Context, hash string) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		return 0, fmt.Errorf("session file removed before revocation: %w", err)
	}
	digest := sha256.Sum256(bytes.TrimSuffix(data, []byte("\n")))
	if hash != s.hash || hash != hex.EncodeToString(digest[:]) {
		return 0, errors.New("revocation targeted another session")
	}
	if s.revokeError {
		return 0, errors.New("synthetic revocation failure")
	}
	s.live = false
	return 1, nil
}

func TestAdminSession_UncertainCommitRevokesBeforeRemovingFile(t *testing.T) {
	for _, cancel := range []bool{false, true} {
		t.Run(fmt.Sprintf("cancelled-%v", cancel), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "session")
			s := newAdminSessionCommitFixture(path)
			s.commitError = true
			ctx, stop := context.WithCancel(context.Background())
			defer stop()
			if cancel {
				s.cancel = stop
			}
			var out, stderr bytes.Buffer
			err := runAdmin(ctx, []string{"session", "create", "--account", "42", "--out", path}, s, &out)
			if err == nil || exitCode(err, &stderr) != 1 || stderr.String() != "server: create session failed\n" || out.Len() != 0 {
				t.Fatal("uncertain commit did not return a fixed error")
			}
			if s.live {
				t.Fatal("uncertain commit left a live session")
			}
			if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
				t.Fatal("revoked session file was not removed")
			}
		})
	}
}

func TestAdminSession_FailedReconciliationRetainsFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session")
	s := newAdminSessionCommitFixture(path)
	s.commitError, s.revokeError = true, true
	var out, stderr bytes.Buffer
	err := runAdmin(context.Background(), []string{"session", "create", "--account", "42", "--out", path}, s, &out)
	if err == nil || exitCode(err, &stderr) != 1 || stderr.String() != "server: revoke session failed; session file retained\n" || out.Len() != 0 {
		t.Fatal("reconciliation failure did not return a fixed error")
	}
	if data, readErr := os.ReadFile(path); readErr != nil || len(data) != 44 || !s.live {
		t.Fatal("unreconciled session lost its file")
	}
}
