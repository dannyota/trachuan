package cli

import (
	"context"
	"fmt"
	"io"
	"os"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/config"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/embed"
	"danny.vn/trachuan/internal/embedding"
	"github.com/jackc/pgx/v5/pgxpool"
)

func runEmbedIndex(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) != 0 {
		_, _ = fmt.Fprintln(stderr, "usage: corpus embed-index")
		return 2
	}
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	db, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	defer db.Close()
	s := embed.New(nil, embedding.DocumentRecipe1())
	if err = s.Backfill(ctx, stage.Env{DB: db, Artifacts: artifact.Store{Root: cfg.Artifacts}}, stdout); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}
