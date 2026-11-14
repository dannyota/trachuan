package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"danny.vn/trachuan/apps/server/internal/seal"
	"danny.vn/trachuan/apps/server/internal/store"
)

func runAccountKeys(ctx context.Context, args []string, q *store.Queries, out io.Writer) error {
	if len(args) != 1 || (args[0] != "check" && args[0] != "reseal") {
		return errors.New("expected account keys check or account keys reseal")
	}
	ring, err := seal.LoadFile(os.Getenv("SERVER_ACCOUNT_KEYS_FILE"))
	if err != nil {
		return err
	}
	if args[0] == "reseal" {
		if err = q.ResealAccountKeys(ctx, ring); err != nil {
			return errors.New("account keys reseal failed")
		}
		return nil
	}
	counts, err := q.CheckAccountKeys(ctx, ring)
	if err != nil {
		return errors.New("account keys check failed")
	}
	for _, table := range []string{"totp_credential", "totp_enrollment", "mail_outbox"} {
		if _, err = fmt.Fprintf(out, "%s %d\n", table, counts[table]); err != nil {
			return fmt.Errorf("write key counts: %w", err)
		}
	}
	return nil
}
