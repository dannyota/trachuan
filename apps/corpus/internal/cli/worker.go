package cli

import (
	"context"
	"fmt"
	"io"
	"os"

	"danny.vn/trachuan/apps/corpus/internal/config"
	"danny.vn/trachuan/apps/corpus/internal/worker"
)

func runWorker(ctx context.Context, args []string, _ io.Writer, stderr io.Writer) int {
	if len(args) != 0 {
		_, _ = fmt.Fprintln(stderr, "usage: corpus worker")
		return 2
	}
	for _, key := range []string{"CORPUS_OCR_STORE", "CORPUS_FILE_CACHE"} {
		info, statErr := os.Stat(os.Getenv(key))
		if os.Getenv(key) == "" || statErr != nil || !info.IsDir() {
			_, _ = fmt.Fprintf(stderr, "%s must name an existing directory\n", key)
			return 1
		}
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
