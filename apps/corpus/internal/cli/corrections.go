package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/stage/build"
	"github.com/jackc/pgx/v5/pgxpool"
)

func runCorrections(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) != 3 || args[0] != "check" {
		_, _ = fmt.Fprintln(stderr, "usage: corpus corrections check <cc> <rel>")
		return 2
	}
	name, err := build.DatabaseName(args[1], args[2])
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 2
	}
	path := filepath.Join("sources", args[1]+"-corrections.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "read corrections %q: %v\n", path, err)
		return 1
	}
	entries, err := source.DecodeCorrections(data)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	url := os.Getenv("CORPUS_BUILDER_DATABASE_URL")
	if url == "" {
		_, _ = fmt.Fprintln(stderr, "CORPUS_BUILDER_DATABASE_URL is required")
		return 1
	}
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "invalid CORPUS_BUILDER_DATABASE_URL")
		return 1
	}
	cfg.ConnConfig.Database = name
	cfg.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "open corpus database failed")
		return 1
	}
	defer db.Close()
	failures, err := build.CheckCorrections(ctx, db, entries)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	for _, failure := range failures {
		_, _ = fmt.Fprintln(stderr, failure)
	}
	if len(failures) != 0 {
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "validated %d corrections\n", len(entries))
	return 0
}
