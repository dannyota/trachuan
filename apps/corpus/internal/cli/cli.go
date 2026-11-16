package cli

import (
	"context"
	"fmt"
	"io"
)

type command func(context.Context, []string, io.Writer, io.Writer) int

func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	commands := map[string]command{"heading-scan": runHeadingScan, "heading-exclusions": runHeadingExclusions, "heading-reviews": runHeadingReviews, "submissions": runSubmissions, "classify": runClassify, "frameworks": runFrameworks, "corrections": runCorrections, "release-row": runReleaseRow, "embed-index": runEmbedIndex, "heading-corrections": runHeadingCorrections, "taxonomy": runTaxonomy, "ocr": runOCR, "source-resume": runSourceResume, "inventory": runInventory, "eval": runEval, "migrate": runMigrate, "worker": runWorker, "start": runStart, "status": runStatus, "cancel": runCancel, "recover": runRecover, "gaps": runGaps, "accept-gaps": runAcceptGaps, "export-history": runExportHistory}
	if len(args) > 0 {
		if run, ok := commands[args[0]]; ok {
			return run(ctx, args[1:], stdout, stderr)
		}
	}
	_, _ = fmt.Fprintln(stderr, "usage: corpus <heading-scan|heading-exclusions|heading-reviews|submissions|classify|frameworks|corrections|release-row|embed-index|heading-corrections|taxonomy|ocr|source-resume|inventory|eval|migrate|worker|start|status|cancel|recover|gaps|accept-gaps|export-history>")
	return 2
}
