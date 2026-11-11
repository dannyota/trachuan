package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"
	"unicode"

	"danny.vn/trachuan/apps/server/internal/store"
)

type reportIDDecider interface {
	decideReportIDs(context.Context, store.DecideReportsParams) (int64, error)
}

type reportDetails interface {
	TriageReports(context.Context, store.TriageReportsParams) ([]store.IssueReport, error)
	DecideReports(context.Context, store.DecideReportsParams) (int64, error)
}

func selectReportGroup(groups []store.TriageGroupsRow, selector string, hash func(store.TriageGroupsRow) string) (store.TriageGroupsRow, error) {
	var found store.TriageGroupsRow
	if len(selector) < 12 || len(selector) > 64 {
		return found, errors.New("invalid group selector")
	}
	count := 0
	for _, g := range groups {
		if strings.HasPrefix(hash(g), selector) {
			found = g
			count++
		}
	}
	if count != 1 {
		return store.TriageGroupsRow{}, errors.New("unknown or ambiguous group")
	}
	return found, nil
}
func findReportGroup(ctx context.Context, s triageStore, selector string, hash func(store.TriageGroupsRow) string) (store.TriageGroupsRow, error) {
	groups, err := s.TriageGroups(ctx, store.TriageGroupsParams{})
	if err != nil {
		return store.TriageGroupsRow{}, fmt.Errorf("list reports: %w", err)
	}
	return selectReportGroup(groups, selector, hash)
}
func showReports(ctx context.Context, s triageStore, args []string, out io.Writer, hash func(store.TriageGroupsRow) string) error {
	if len(args) < 1 || len(args) > 2 || (len(args) == 2 && args[1] != "--accounts") {
		return errors.New("invalid show flags")
	}
	d, ok := s.(reportDetails)
	if !ok {
		return errors.New("reports store unavailable")
	}
	g, err := findReportGroup(ctx, s, args[0], hash)
	if err != nil {
		return err
	}
	rows, err := d.TriageReports(ctx, store.TriageReportsParams{DocumentKey: g.DocumentKey, Anchor: g.Anchor, Kind: g.Kind, Target: g.Target})
	if err != nil {
		return fmt.Errorf("show reports: %w", err)
	}
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	if _, err = fmt.Fprintf(w, "GROUP\tDOCUMENT\tANCHOR\tKIND\tTARGET\tREPORTS\tACCOUNTS\tNEWEST\tSUGGESTIONS\n%s\t%s\t%s\t%s\t%s\t%d\t%d\t%s\t%s\n", groupID(g)[:12], g.DocumentKey, g.Anchor, g.Kind, g.Target, g.ReportCount, g.AccountCount, g.Newest.Format("2006-01-02T15:04:05Z07:00"), strings.Join(g.Suggestions, ", ")); err != nil {
		return err
	}
	header := "ID\tRELEASE\tSTATE\tSUGGESTION"
	if len(args) == 2 {
		header += "\tACCOUNT"
	}
	if _, err = fmt.Fprintln(w, header); err != nil {
		return err
	}
	for _, r := range rows {
		if _, err = fmt.Fprintf(w, "%d\t%s\t%s\t%s", r.ID, r.Release, r.State, r.Suggestion); err != nil {
			return err
		}
		if len(args) == 2 {
			if _, err = fmt.Fprintf(w, "\t%d", r.AccountID); err != nil {
				return err
			}
		}
		if _, err = fmt.Fprintln(w); err != nil {
			return err
		}
	}
	return w.Flush()
}
func decideReports(ctx context.Context, s triageStore, decision string, args []string, out io.Writer, hash func(store.TriageGroupsRow) string) error {
	d, ok := s.(reportDetails)
	if !ok {
		return errors.New("reports store unavailable")
	}
	var selector, by string
	var ids []int64
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--by":
			i++
			if i >= len(args) || by != "" {
				return errors.New("invalid operator")
			}
			by = args[i]
		case "--ids":
			i++
			for ; i < len(args) && !strings.HasPrefix(args[i], "--"); i++ {
				for _, part := range strings.Split(args[i], ",") {
					id, e := strconv.ParseInt(part, 10, 64)
					if e != nil || id <= 0 {
						return errors.New("invalid report id")
					}
					ids = append(ids, id)
				}
			}
			i--
		default:
			if selector != "" || strings.HasPrefix(args[i], "--") {
				return errors.New("invalid selector")
			}
			selector = args[i]
		}
	}
	if by == "" || len(by) > 200 || strings.TrimSpace(by) == "" {
		return errors.New("invalid operator")
	}
	for _, r := range by {
		if !unicode.IsPrint(r) {
			return errors.New("invalid operator")
		}
	}
	if (selector == "") == (len(ids) == 0) {
		return errors.New("invalid selector")
	}
	p := store.DecideReportsParams{DecidedBy: &by, Ids: ids, Decision: map[string]string{"accept": "accepted", "reject": "rejected"}[decision]}
	if selector != "" {
		g, err := findReportGroup(ctx, s, selector, hash)
		if err != nil {
			return err
		}
		p.GroupDocumentKey = &g.DocumentKey
		p.GroupAnchor = &g.Anchor
		p.GroupKind = &g.Kind
		p.GroupTarget = &g.Target
	}
	var n int64
	var err error
	if len(ids) > 0 {
		decider, ok := s.(reportIDDecider)
		if !ok {
			return errors.New("reports store unavailable")
		}
		n, err = decider.decideReportIDs(ctx, p)
	} else {
		n, err = d.DecideReports(ctx, p)
	}
	if err != nil {
		return fmt.Errorf("decide reports: %w", err)
	}
	_, err = fmt.Fprintln(out, n)
	return err
}
