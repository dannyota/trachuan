package cli

import (
	"context"
	"fmt"
	"io"
	"os"

	"danny.vn/trachuan/apps/corpus/internal/config"
	"danny.vn/trachuan/apps/corpus/internal/migrate"
)

func runMigrate(ctx context.Context, args []string, _ io.Writer, stderr io.Writer) int {
	if len(args) != 0 {
		_, _ = fmt.Fprintln(stderr, "usage: corpus migrate")
		return 2
	}
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "load configuration: %v\n", err)
		return 1
	}
	if err := migrate.Up(ctx, cfg.DatabaseURL); err != nil {
		_, _ = fmt.Fprintf(stderr, "migrate workshop: %v\n", err)
		return 1
	}
	return 0
}
