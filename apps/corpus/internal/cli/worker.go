package cli

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/dannyota/traquydinh/apps/corpus/internal/config"
	"github.com/dannyota/traquydinh/apps/corpus/internal/worker"
)

func runWorker(ctx context.Context, args []string, _ io.Writer, stderr io.Writer) int {
	if len(args) != 0 {
		_, _ = fmt.Fprintln(stderr, "usage: corpus worker")
		return 2
	}
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "load configuration: %v\n", err)
		return 1
	}
	if err = worker.Run(ctx, cfg); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}
