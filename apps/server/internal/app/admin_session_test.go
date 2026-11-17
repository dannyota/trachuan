package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/store"
	"github.com/jackc/pgx/v5"
)

type sessionAccountFixture struct {
	adminStore
	account store.Account
	err     error
}

func (s sessionAccountFixture) GetAccount(context.Context, int64) (store.Account, error) {
	return s.account, s.err
}

func TestAdminSession_RefusesUnsafeCreation(t *testing.T) {
	now := time.Now()
	synthetic, real := "loadcheck:unit", "google-subject"
	for _, tc := range []struct {
		name, hours, want string
		sub               *string
		blocked, deleting bool
		existing, missing bool
		lookupErr         error
	}{
		{name: "real subject", sub: &real, want: "session requires a loadcheck account"},
		{name: "no subject", want: "session requires a loadcheck account"},
		{name: "blocked", sub: &synthetic, blocked: true, want: "account is blocked"},
		{name: "deleting", sub: &synthetic, deleting: true, want: "account is being deleted"},
		{name: "existing file", sub: &synthetic, existing: true, want: "create session file failed"},
		{name: "missing parent", sub: &synthetic, missing: true, want: "create session file failed"},
		{name: "zero hours", sub: &synthetic, hours: "0", want: "invalid session fields"},
		{name: "too many hours", sub: &synthetic, hours: "25", want: "invalid session fields"},
		{name: "unknown account", lookupErr: pgx.ErrNoRows, want: "account not found"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "session")
			if tc.existing {
				if err := os.WriteFile(path, []byte("keep\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if tc.missing {
				path = filepath.Join(path, "session")
			}
			s := sessionAccountFixture{account: store.Account{ID: 42, GoogleSub: tc.sub}, err: tc.lookupErr}
			if tc.blocked {
				s.account.BlockedAt = &now
			}
			if tc.deleting {
				s.account.DeleteRequestedAt = &now
			}
			args := []string{"session", "create", "--account", "42", "--out", path}
			if tc.hours != "" {
				args = append(args, "--hours", tc.hours)
			}
			var out, stderr bytes.Buffer
			err := runAdmin(context.Background(), args, s, &out)
			if err == nil || err.Error() != tc.want {
				t.Fatalf("error=%v, want %s", err, tc.want)
			}
			if exitCode(err, &stderr) != 1 || out.Len() != 0 {
				t.Fatal("unexpected command output")
			}
			if tc.existing {
				data, readErr := os.ReadFile(path)
				if readErr != nil || string(data) != "keep\n" {
					t.Fatal("existing file changed")
				}
			} else if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
				t.Fatal("refused command left a file")
			}
		})
	}
}

func TestAdminSession_RejectsInvalidFlags(t *testing.T) {
	for _, args := range [][]string{
		{"session"}, {"session", "unknown"}, {"session", "create"},
		{"session", "create", "--account", "0", "--out", "unused"},
		{"session", "create", "--account", "42"},
		{"session", "create", "--account", "42", "--out", "unused", "--hours", "bad"},
		{"session", "create", "--account", "42", "--out", "unused", "trailing"},
	} {
		if err := runAdmin(context.Background(), args, nil, &bytes.Buffer{}); err == nil || err.Error() != "invalid session fields" {
			t.Fatalf("args=%q error=%v", args, err)
		}
	}
}
