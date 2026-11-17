package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
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
	afterRevoke                    func() error
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
	if s.afterRevoke != nil {
		if changeErr := s.afterRevoke(); changeErr != nil {
			return 0, changeErr
		}
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

func TestAdminSession_RevokeErrorStillRemovesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session")
	s := newAdminSessionCommitFixture(path)
	s.commitError, s.revokeError = true, true
	var out, stderr bytes.Buffer
	err := runAdmin(context.Background(), []string{"session", "create", "--account", "42", "--out", path}, s, &out)
	if err == nil || exitCode(err, &stderr) != 1 || stderr.String() != "server: create session failed\n" || out.Len() != 0 {
		t.Fatal("reconciliation failure did not return a fixed error")
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatal("revocation failure left the session value on disk")
	}
}

func TestAdminSession_RemoveErrorClearsHeldFile(t *testing.T) {
	for _, revokeError := range []bool{false, true} {
		t.Run(fmt.Sprintf("revoke-error-%v", revokeError), func(t *testing.T) {
			parent := t.TempDir()
			directory, moved := filepath.Join(parent, "original"), filepath.Join(parent, "moved")
			if err := os.Mkdir(directory, 0700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(directory, "session")
			s := newAdminSessionCommitFixture(path)
			s.commitError, s.revokeError = true, revokeError
			s.afterRevoke = func() error { return os.Rename(directory, moved) }
			var out, stderr bytes.Buffer
			err := runAdmin(context.Background(), []string{"session", "create", "--account", "42", "--out", path}, s, &out)
			if err == nil || exitCode(err, &stderr) != 1 || stderr.String() != "server: remove session file failed\n" || out.Len() != 0 {
				t.Fatal("removal failure did not return a fixed error")
			}
			if data, readErr := os.ReadFile(filepath.Join(moved, "session")); readErr != nil || len(data) != 0 {
				t.Fatal("removal failure left a readable session value")
			}
		})
	}
}

func TestAdminSession_RenameThenReplaceClearsOriginalFile(t *testing.T) {
	for _, revokeError := range []bool{false, true} {
		t.Run(fmt.Sprintf("revoke-error-%v", revokeError), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "session")
			moved := path + ".moved"
			s := newAdminSessionCommitFixture(path)
			s.commitError, s.revokeError = true, revokeError
			s.afterRevoke = func() error {
				if err := os.Rename(path, moved); err != nil {
					return err
				}
				return os.WriteFile(path, []byte("synthetic replacement\n"), 0600)
			}
			var out, stderr bytes.Buffer
			err := runAdmin(context.Background(), []string{"session", "create", "--account", "42", "--out", path}, s, &out)
			if err == nil || exitCode(err, &stderr) != 1 || stderr.String() != "server: create session failed\n" || out.Len() != 0 {
				t.Fatal("renamed session did not return a fixed error")
			}
			if data, readErr := os.ReadFile(moved); readErr != nil || len(data) != 0 {
				t.Fatal("pathname replacement left the original session value readable")
			}
			if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
				t.Fatal("replacement pathname was not removed")
			}
		})
	}
}

func TestAdminSession_SuccessWritesOnlyValue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session")
	s := newAdminSessionCommitFixture(path)
	var out, stderr bytes.Buffer
	err := runAdmin(context.Background(), []string{"session", "create", "--account", "42", "--out", path}, s, &out)
	if exitCode(err, &stderr) != 0 || out.Len() != 0 || stderr.Len() != 0 {
		t.Fatal("successful session creation wrote diagnostics")
	}
	data, readErr := os.ReadFile(path)
	if readErr != nil || len(data) != 44 || data[43] != '\n' {
		t.Fatal("session file does not contain exactly a cookie value and newline")
	}
	if decoded, decodeErr := base64.RawURLEncoding.Strict().DecodeString(string(data[:43])); decodeErr != nil || len(decoded) != 32 {
		t.Fatal("session value is not 32 bytes")
	}
	digest := sha256.Sum256(data[:43])
	if hex.EncodeToString(digest[:]) != s.hash || !s.live {
		t.Fatal("session file does not authenticate the committed session")
	}
}

func TestAdminSession_ClearFailureReportsPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session")
	if err := os.WriteFile(path, []byte("synthetic-session-value\n"), 0600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	if err = os.Rename(path, path+".moved"); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(path, "entry"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if clearErr := clearAdminSessionFile(file); clearErr == nil || clearErr.Error() != fmt.Sprintf("session file could not be cleared: %q", path) {
		t.Fatal("clearing failure did not name the path without its value")
	}
}
