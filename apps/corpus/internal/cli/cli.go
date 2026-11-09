// Package cli dispatches corpus commands.
package cli

import (
	"context"
	"fmt"
	"io"
)

type command func(context.Context, []string, io.Writer, io.Writer) int

// Run dispatches a command and returns its exit status.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	commands := map[string]command{"inventory": runInventory, "eval": runEval, "migrate": runMigrate, "worker": runWorker, "start": runStart, "status": runStatus, "cancel": runCancel, "recover": runRecover, "gaps": runGaps, "accept-gaps": runAcceptGaps, "export-history": runExportHistory}
	if len(args) > 0 {
		if run, ok := commands[args[0]]; ok {
			return run(ctx, args[1:], stdout, stderr)
		}
	}
	_, _ = fmt.Fprintln(stderr, "usage: corpus <inventory|eval|migrate|worker|start|status|cancel|recover|gaps|accept-gaps|export-history>")
	return 2
}
