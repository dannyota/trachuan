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
	"github.com/jackc/pgx/v5"
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

type reportIDReader interface {
	reportsByIDs(context.Context, []int64) ([]store.IssueReport, error)
}

func (s credentialStore) reportsByIDs(ctx context.Context, ids []int64) ([]store.IssueReport, error) {
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, fmt.Errorf("begin report lookup: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	rows, err := tx.Query(ctx, "SELECT id, account_id, release, state, suggestion, document_key, anchor, kind, target FROM issue_report WHERE id = ANY($1::bigint[]) ORDER BY id", ids)
	if err != nil {
		return nil, fmt.Errorf("find reports: %w", err)
	}
	defer rows.Close()
	var result []store.IssueReport
	for rows.Next() {
		var row store.IssueReport
		if err := rows.Scan(&row.ID, &row.AccountID, &row.Release, &row.State, &row.Suggestion, &row.DocumentKey, &row.Anchor, &row.Kind, &row.Target); err != nil {
			return nil, fmt.Errorf("read reports: %w", err)
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read reports: %w", err)
	}
	return result, nil
}

func showReports(ctx context.Context, s triageStore, args []string, out io.Writer, hash func(store.TriageGroupsRow) string) error {
	var selector string
	var ids []int64
	accounts, idsFlag := false, false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--accounts":
			if accounts {
				return errors.New("invalid show flags")
			}
			accounts = true
		case "--ids":
			if idsFlag {
				return errors.New("invalid show flags")
			}
			idsFlag = true
			for i++; i < len(args) && !strings.HasPrefix(args[i], "--"); i++ {
				for _, part := range strings.Split(args[i], ",") {
					id, err := strconv.ParseInt(part, 10, 64)
					if err != nil || id <= 0 {
						return errors.New("invalid report id")
					}
					ids = append(ids, id)
				}
			}
			i--
		default:
			if selector != "" || strings.HasPrefix(args[i], "--") {
				return errors.New("invalid show flags")
			}
			selector = args[i]
		}
	}
	if idsFlag && (len(ids) == 0 || selector != "") || !idsFlag && selector == "" {
		return errors.New("invalid show flags")
	}
	var rows []store.IssueReport
	var g store.TriageGroupsRow
	var err error
	if idsFlag {
		reader, ok := s.(reportIDReader)
		if !ok {
			return errors.New("reports store unavailable")
		}
		rows, err = reader.reportsByIDs(ctx, ids)
		if err != nil {
			return fmt.Errorf("show reports: %w", err)
		}
		found := make(map[int64]bool, len(rows))
		for _, row := range rows {
			found[row.ID] = true
		}
		for _, id := range ids {
			if !found[id] {
				return errors.New("unknown report id")
			}
		}
	} else {
		d, ok := s.(reportDetails)
		if !ok {
			return errors.New("reports store unavailable")
		}
		g, err = findReportGroup(ctx, s, selector, hash)
		if err != nil {
			return err
		}
		rows, err = d.TriageReports(ctx, store.TriageReportsParams{DocumentKey: g.DocumentKey, Anchor: g.Anchor, Kind: g.Kind, Target: g.Target})
		if err != nil {
			return fmt.Errorf("show reports: %w", err)
		}
	}
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	if !idsFlag {
		if _, err = fmt.Fprintf(w, "GROUP\tDOCUMENT\tANCHOR\tKIND\tTARGET\tREPORTS\tACCOUNTS\tNEWEST\tSUGGESTIONS\n%s\t%s\t%s\t%s\t%s\t%d\t%d\t%s\t%s\n", groupID(g)[:12], g.DocumentKey, g.Anchor, g.Kind, g.Target, g.ReportCount, g.AccountCount, g.Newest.Format("2006-01-02T15:04:05Z07:00"), strings.Join(g.Suggestions, ", ")); err != nil {
			return err
		}
	}
	header := "ID\tRELEASE\tSTATE\tSUGGESTION"
	if idsFlag {
		header += "\tDOCUMENT\tANCHOR\tKIND\tTARGET"
	}
	if accounts {
		header += "\tACCOUNT"
	}
	if _, err = fmt.Fprintln(w, header); err != nil {
		return err
	}
	for _, r := range rows {
		if _, err = fmt.Fprintf(w, "%d\t%s\t%s\t%s", r.ID, r.Release, r.State, r.Suggestion); err != nil {
			return err
		}
		if idsFlag {
			if _, err = fmt.Fprintf(w, "\t%s\t%s\t%s\t%s", r.DocumentKey, r.Anchor, r.Kind, r.Target); err != nil {
				return err
			}
		}
		if accounts {
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
