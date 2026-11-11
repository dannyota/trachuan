package app

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"time"

	"danny.vn/trachuan/apps/server/internal/admin"
)

type accountEvicter interface{ EvictAccount(int64) }

func runAdmin(ctx context.Context, args []string, queries admin.Store, out io.Writer, caches ...accountEvicter) error {
	if len(args) < 2 {
		return errors.New("missing admin subcommand")
	}
	if args[0] == "reports" {
		return runReports(ctx, args[1:], queries, out)
	}
	service := &admin.Service{Store: queries}
	flags := flag.NewFlagSet("admin", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	account := flags.Int64("account", 0, "account ID")
	googleSub := flags.String("google-sub", "", "Google subject")
	email := flags.String("email", "", "email")
	name := flags.String("name", "", "token name")
	days := flags.Int("days", 90, "token lifetime")
	hash := flags.String("hash", "", "token hash")
	if err := flags.Parse(args[2:]); err != nil || flags.NArg() != 0 {
		return errors.New("invalid admin flags")
	}
	switch args[0] + " " + args[1] {
	case "account create":
		if *googleSub == "" || *email == "" {
			return errors.New("missing account fields")
		}
		row, err := service.CreateAccount(ctx, *googleSub, *email)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(out, row.ID)
		return err
	case "account block":
		if *account <= 0 {
			return errors.New("invalid account")
		}
		err := service.BlockAccount(ctx, *account)
		for _, cache := range caches {
			cache.EvictAccount(*account)
		}
		return err
	case "token create":
		if *account <= 0 || *name == "" || *days < 1 || *days > 365 {
			return errors.New("invalid token fields")
		}
		token, err := service.CreateToken(ctx, *account, *name, time.Duration(*days)*24*time.Hour)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(out, token)
		return err
	case "token revoke":
		return service.RevokeToken(ctx, *hash)
	default:
		return errors.New("unknown admin subcommand")
	}
}
