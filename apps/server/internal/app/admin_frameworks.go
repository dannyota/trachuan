package app

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"strings"
	"time"

	"danny.vn/trachuan/apps/server/internal/accountemail"
	"danny.vn/trachuan/apps/server/internal/store"
)

type frameworkAdminStore interface {
	AllowFrameworkEntitlement(context.Context, store.FrameworkAllowance) (store.FrameworkEntitlement, error)
	RevokeFrameworkEntitlement(context.Context, store.FrameworkRevocation) error
	ListFrameworkEntitlements(context.Context, string) ([]store.FrameworkEntitlement, error)
}

type frameworkFlags struct {
	email, scope, by, note string
	notes                  bool
}

func parseFrameworkFlags(args []string) (frameworkFlags, error) {
	var p frameworkFlags
	invalid := errors.New("invalid frameworks command")
	if len(args) == 0 {
		return p, invalid
	}
	flags := flag.NewFlagSet("frameworks", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&p.email, "email", "", "email")
	switch args[0] {
	case "allow":
		flags.StringVar(&p.note, "note", "", "operator note")
		fallthrough
	case "revoke":
		flags.StringVar(&p.scope, "scope", "", "framework scope")
		flags.StringVar(&p.by, "by", "", "operator name")
	case "list":
		flags.BoolVar(&p.notes, "notes", false, "include operator notes")
	default:
		return p, invalid
	}
	if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 {
		return p, invalid
	}
	emailPresent := false
	flags.Visit(func(f *flag.Flag) {
		if f.Name == "email" {
			emailPresent = true
		}
	})
	if args[0] != "list" || emailPresent {
		email, err := accountemail.Parse(p.email)
		if err != nil {
			return p, invalid
		}
		p.email = email
	}
	if args[0] != "list" {
		if _, err := store.ParseFrameworkScope(p.scope); err != nil || strings.TrimSpace(p.by) == "" {
			return p, invalid
		}
	}
	return p, nil
}

func runFrameworks(ctx context.Context, args []string, queries adminStore, out io.Writer) error {
	p, err := parseFrameworkFlags(args)
	if err != nil {
		return err
	}
	q, ok := queries.(frameworkAdminStore)
	if !ok {
		return errors.New("framework entitlement store unavailable")
	}
	switch args[0] {
	case "allow":
		_, err = q.AllowFrameworkEntitlement(ctx, store.FrameworkAllowance{Email: p.email, Scope: p.scope, By: p.by, Note: p.note})
	case "revoke":
		err = q.RevokeFrameworkEntitlement(ctx, store.FrameworkRevocation{Email: p.email, Scope: p.scope, By: p.by})
	case "list":
		var rows []store.FrameworkEntitlement
		rows, err = q.ListFrameworkEntitlements(ctx, p.email)
		if err == nil {
			return printFrameworkEntitlements(out, rows, p.notes)
		}
	}
	if errors.Is(err, store.ErrFrameworkAlreadyAllowed) || errors.Is(err, store.ErrFrameworkNotAllowed) {
		return err
	}
	if err != nil {
		return errors.New("framework entitlement store unavailable")
	}
	return nil
}

func printFrameworkEntitlements(out io.Writer, rows []store.FrameworkEntitlement, notes bool) error {
	encoder := json.NewEncoder(out)
	for _, row := range rows {
		record := struct {
			ID        int64     `json:"id"`
			Email     string    `json:"email"`
			Scope     string    `json:"scope"`
			GrantedAt time.Time `json:"granted_at"`
			GrantedBy string    `json:"granted_by"`
			Note      *string   `json:"note,omitempty"`
		}{ID: row.ID, Email: row.Email, Scope: row.Scope, GrantedAt: row.GrantedAt, GrantedBy: row.GrantedBy}
		if notes {
			record.Note = &row.Note
		}
		if err := encoder.Encode(record); err != nil {
			return err
		}
	}
	return nil
}
