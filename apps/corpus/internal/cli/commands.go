package cli

import (
	"context"
	"fmt"
	"io"

	"danny.vn/traquydinh/apps/corpus/internal/run"
)

func runStart(ctx context.Context, a []string, out, errout io.Writer) int {
	return lifecycle(ctx, "start", a, out, errout)
}
func runStatus(ctx context.Context, a []string, out, errout io.Writer) int {
	return lifecycle(ctx, "status", a, out, errout)
}
func runCancel(ctx context.Context, a []string, out, errout io.Writer) int {
	return lifecycle(ctx, "cancel", a, out, errout)
}
func runRecover(ctx context.Context, a []string, out, errout io.Writer) int {
	return lifecycle(ctx, "recover", a, out, errout)
}
func runGaps(ctx context.Context, a []string, out, errout io.Writer) int {
	return lifecycle(ctx, "gaps", a, out, errout)
}
func runAcceptGaps(ctx context.Context, a []string, out, errout io.Writer) int {
	return lifecycle(ctx, "accept-gaps", a, out, errout)
}
func runExportHistory(ctx context.Context, args []string, _ io.Writer, stderr io.Writer) int {
	if len(args) != 3 {
		_, _ = fmt.Fprintln(stderr, "usage: corpus export-history <workflow-id> <run-id> <path>")
		return 2
	}
	d, closeDeps, err := dependencies(ctx, false, "")
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	defer closeDeps()
	if err = run.ExportHistory(ctx, d, args[0], args[1], args[2]); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}
