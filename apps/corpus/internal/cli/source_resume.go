package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/run"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"github.com/jackc/pgx/v5"
	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
)

func runSourceResume(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) < 2 {
		_, _ = fmt.Fprintln(stderr, "usage: corpus source-resume <cc> <rel> --source <name> --note <reason>")
		return 2
	}
	opts, err := options(args[2:], map[string]bool{"--source": true, "--note": true})
	if err != nil || (opts["--source"] != "vbpl" && opts["--source"] != "chinhphu") || strings.TrimSpace(opts["--note"]) == "" {
		_, _ = fmt.Fprintln(stderr, "source-resume requires --source vbpl|chinhphu and --note <reason>")
		return 2
	}
	d, closeDeps, err := dependencies(ctx, false, args[0])
	if err == nil {
		defer closeDeps()
		err = resumeSource(ctx, d, args[0], args[1], opts["--source"], opts["--note"], stdout)
	}
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

func resumeSource(ctx context.Context, d run.Deps, cc, rel, source, note string, out io.Writer) error {
	tx, err := d.DB.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin source resume: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := store.New(tx)
	row, err := q.GetRunForUpdate(ctx, store.GetRunForUpdateParams{Country: cc, Rel: rel})
	if err != nil {
		return fmt.Errorf("lock run: %w", err)
	}
	if row.State == "released" {
		return errors.New("cannot resume source in released run")
	}
	desc, err := d.Temporal.DescribeWorkflowExecution(ctx, run.WorkflowID(cc, rel), "")
	var missing *serviceerror.NotFound
	if err != nil && !errors.As(err, &missing) {
		return fmt.Errorf("describe workflow: %w", err)
	}
	if err == nil && desc.WorkflowExecutionInfo.GetStatus() == enumspb.WORKFLOW_EXECUTION_STATUS_RUNNING {
		return errors.New("cannot resume source with open workflow")
	}
	state, err := q.GetRunSourceForUpdate(ctx, store.GetRunSourceForUpdateParams{RunID: row.ID, Source: source})
	if errors.Is(err, pgx.ErrNoRows) {
		_, err = fmt.Fprintf(out, "%s is already active\n", source)
		return err
	}
	if err != nil {
		return fmt.Errorf("lock source: %w", err)
	}

	reason := ""
	if state.State == "source_paused" {
		if state.PauseReason != nil {
			reason = *state.PauseReason
		}
		n, e := q.ClearRunSource(ctx, store.ClearRunSourceParams{RunID: row.ID, Source: source})
		if e != nil {
			return fmt.Errorf("clear source: %w", e)
		}
		if n != 1 {
			return errors.New("source pause changed")
		}
		if e = q.InsertRunSourceEvent(ctx, store.InsertRunSourceEventParams{RunID: row.ID, Source: source, Event: "clear", ClearNote: &note}); e != nil {
			return fmt.Errorf("record source resume: %w", e)
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit source resume: %w", err)
	}
	if state.State == "source_paused" {
		_, err = fmt.Fprintf(out, "resumed %s; previous reason: %s\n", source, reason)
	} else {
		_, err = fmt.Fprintf(out, "%s is already active\n", source)
	}
	return err
}
