package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"danny.vn/trachuan/apps/server/internal/admin"
	"danny.vn/trachuan/apps/server/internal/store"
)

type triageStore interface {
	TriageGroups(context.Context, store.TriageGroupsParams) ([]store.TriageGroupsRow, error)
}

func groupID(g store.TriageGroupsRow) string {
	h := sha256.Sum256([]byte(strings.Join([]string{g.DocumentKey, g.Anchor, g.Kind, g.Target}, "\x1f")))
	return hex.EncodeToString(h[:])
}

func runReports(ctx context.Context, args []string, base admin.Store, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("missing reports subcommand")
	}
	s, ok := base.(triageStore)
	if !ok {
		return errors.New("reports store unavailable")
	}
	switch args[0] {
	case "list":
		f := flag.NewFlagSet("list", flag.ContinueOnError)
		f.SetOutput(io.Discard)
		kind := f.String("kind", "", "kind")
		state := f.String("state", "", "state")
		if f.Parse(args[1:]) != nil || f.NArg() != 0 {
			return errors.New("invalid reports flags")
		}
		var p store.TriageGroupsParams
		if *kind != "" {
			p.FilterKind = kind
		}
		if *state != "" {
			p.FilterState = state
		}
		groups, err := s.TriageGroups(ctx, p)
		if err != nil {
			return fmt.Errorf("list reports: %w", err)
		}
		w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
		if _, err = fmt.Fprintln(w, "GROUP\tDOCUMENT\tANCHOR\tKIND\tTARGET\tREPORTS\tACCOUNTS\tNEWEST\tSUGGESTIONS"); err != nil {
			return err
		}
		for _, g := range groups {
			if _, err = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%d\t%d\t%s\t%s\n", groupID(g)[:12], g.DocumentKey, g.Anchor, g.Kind, g.Target, g.ReportCount, g.AccountCount, g.Newest.Format("2006-01-02T15:04:05Z07:00"), strings.Join(g.Suggestions, ", ")); err != nil {
				return err
			}
		}
		return w.Flush()
	case "show":
		return showReports(ctx, s, args[1:], out, groupID)
	case "accept", "reject":
		return decideReports(ctx, s, args[0], args[1:], out, groupID)
	case "export":
		return exportReports(ctx, base, args[1:], out)
	default:
		return errors.New("unknown reports subcommand")
	}
}
