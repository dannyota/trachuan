package cli

import (
	"context"
	"fmt"
	"io"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/classify"
)

func runSubmissions(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	opts, err := options(args, map[string]bool{"--rebuild": false, "--reconcile": true, "--abandon": true})
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 2
	}
	actions := 0
	for _, key := range []string{"--rebuild", "--reconcile", "--abandon"} {
		if opts[key] != "" {
			actions++
		}
	}
	if actions > 1 {
		_, _ = fmt.Fprintln(stderr, "choose one of --rebuild, --reconcile ID or --abandon ID")
		return 2
	}
	runner, db, _, err := openSubmissionRunner(ctx, opts["--reconcile"] != "" || opts["--abandon"] != "")
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	defer db.Close()
	switch {
	case opts["--rebuild"] != "":
		err = runner.Rebuild(ctx)
	case opts["--reconcile"] != "":
		err = runner.Reconcile(ctx, opts["--reconcile"], false)
	case opts["--abandon"] != "":
		err = runner.Reconcile(ctx, opts["--abandon"], true)
	}
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	unlock, err := runner.Repository.Lock(ctx)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	defer unlock()
	rows, err := runner.Repository.List(ctx)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	for _, s := range rows {
		if err = printSubmission(stdout, s); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return 1
		}
	}
	return 0
}

func printSubmission(out io.Writer, s classify.Submission) error {
	_, err := fmt.Fprintf(out, "id=%s release=%s/%s state=%s requests=%d created_at=%s window_end=%s reserved_micro_usd=%d cost_micro_usd=%d settled=%t\n", s.ID, s.Country, s.Release, s.State, len(s.Items), s.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"), s.CreatedAt.Add(10*time.Minute).UTC().Format("2006-01-02T15:04:05Z"), s.ReserveMicroUSD, s.CostMicroUSD, s.Settled)
	if err != nil {
		return fmt.Errorf("write submission summary: %w", err)
	}
	return nil
}

func runSubmissionCancel(ctx context.Context, args []string, stderr io.Writer) int {
	opts, err := options(args, map[string]bool{"--submission": true, "--out": true})
	if err != nil || opts["--submission"] == "" || opts["--out"] == "" {
		_, _ = fmt.Fprintln(stderr, "usage: corpus cancel --submission ID --out DIR")
		return 2
	}
	runner, db, _, err := openSubmissionRunner(ctx, true)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	defer db.Close()
	if err = runner.Cancel(ctx, opts["--submission"], opts["--out"]); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}
