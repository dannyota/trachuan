package app

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"danny.vn/trachuan/apps/server/internal/store"
	"danny.vn/trachuan/apps/server/internal/websession"
	"github.com/jackc/pgx/v5"
)

type adminSessionStore interface {
	withAdminSession(context.Context, int64, int32, func(*store.Queries) error) error
	RevokeSession(context.Context, string) (int64, error)
}

func runAdminSession(ctx context.Context, args []string, queries adminStore) (result error) {
	if len(args) == 0 || args[0] != "create" {
		return errors.New("invalid session fields")
	}
	flags := flag.NewFlagSet("session create", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	id := flags.Int64("account", 0, "account ID")
	path := flags.String("out", "", "session file")
	hours := flags.Int("hours", 24, "session lifetime")
	if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 || *id <= 0 || *path == "" || *hours < 1 || *hours > 24 {
		return errors.New("invalid session fields")
	}
	account, err := queries.GetAccount(ctx, *id)
	if errors.Is(err, pgx.ErrNoRows) {
		return errors.New("account not found")
	}
	if err != nil {
		return errors.New("account store unavailable")
	}
	if checkErr := checkSessionAccount(account); checkErr != nil {
		return checkErr
	}
	file, err := os.OpenFile(*path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return errors.New("create session file failed")
	}
	closed, committed := false, false
	cleanup := file
	defer func() {
		if !committed {
			if clearErr := clearAdminSessionFile(cleanup); clearErr != nil {
				result = clearErr
			}
		}
		if !closed {
			_ = file.Close()
		}
		if cleanup != file {
			_ = cleanup.Close()
		}
	}()
	if modeErr := file.Chmod(0600); modeErr != nil {
		return errors.New("set session file mode failed")
	}
	info, statErr := file.Stat()
	if statErr != nil || info.Mode().Perm() != 0600 {
		return errors.New("verify session file mode failed")
	}
	cleanup, err = duplicateAdminSessionFile(file)
	if err != nil {
		cleanup = file
		return errors.New("prepare session file cleanup failed")
	}
	db, ok := queries.(adminSessionStore)
	if !ok {
		return errors.New("session store unavailable")
	}
	expires := time.Now().Add(time.Duration(*hours) * time.Hour)
	manager := &websession.Manager{}
	token, params, valueErr := manager.NewSessionParams(expires)
	if valueErr != nil {
		return errors.New("create session failed")
	}
	params.AccountID, params.AuthEpoch = account.ID, account.AuthEpoch
	err = db.withAdminSession(ctx, account.ID, account.AuthEpoch, func(q *store.Queries) error {
		if _, createErr := q.CreateSession(ctx, params); createErr != nil {
			return createErr
		}
		if _, writeErr := io.WriteString(file, token+"\n"); writeErr != nil {
			return writeErr
		}
		if syncErr := file.Sync(); syncErr != nil {
			return syncErr
		}
		closed = true
		return file.Close()
	})
	if err != nil {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), authQueryDeadline)
		_, _ = db.RevokeSession(cleanupCtx, params.TokenHash)
		cancel()
		return errors.New("create session failed")
	}
	committed = true
	return nil
}

func clearAdminSessionFile(file *os.File) error {
	if err := os.Remove(file.Name()); err == nil {
		return nil
	}
	if err := file.Truncate(0); err != nil {
		return fmt.Errorf("session file could not be cleared: %q", file.Name())
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("session file could not be cleared: %q", file.Name())
	}
	return errors.New("remove session file failed")
}

func checkSessionAccount(account store.Account) error {
	if account.DeleteRequestedAt != nil {
		return errors.New("account is being deleted")
	}
	if account.BlockedAt != nil {
		return errors.New("account is blocked")
	}
	if account.GoogleSub == nil || !strings.HasPrefix(*account.GoogleSub, "loadcheck:") {
		return errors.New("session requires a loadcheck account")
	}
	return nil
}

func (s credentialStore) withAdminSession(ctx context.Context, id int64, epoch int32, write func(*store.Queries) error) error {
	return s.withAccount(ctx, id, epoch, func(q *store.Queries) error {
		account, err := q.GetAccount(ctx, id)
		if err != nil {
			return err
		}
		if checkErr := checkSessionAccount(account); checkErr != nil {
			return checkErr
		}
		enrolled, err := q.HasTOTP(ctx, id)
		if err != nil {
			return err
		}
		if enrolled {
			return store.ErrFactorEnrolled
		}
		return write(q)
	})
}
