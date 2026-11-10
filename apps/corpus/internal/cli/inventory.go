package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/config"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/build"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

func runInventory(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) < 2 {
		_, _ = fmt.Fprintln(stderr, "usage: corpus inventory <cc> <rel> --out FILE")
		return 2
	}
	opts, err := options(args[2:], map[string]bool{"--out": true})
	if err != nil || opts["--out"] == "" {
		_, _ = fmt.Fprintln(stderr, "--out FILE is required")
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
	q := store.New(db)
	row, err := q.GetRun(ctx, store.GetRunParams{Country: args[0], Rel: args[1]})
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	var snapshot stage.Snapshot
	if err = json.Unmarshal(row.Snapshot, &snapshot); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	inv, err := build.LoadInventory(ctx, q, stage.Run{ID: row.ID, Country: row.Country, Rel: row.Rel, Snapshot: snapshot}, "relate")
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	if _, err = build.IdentityInventory(inv, artifact.Store{Root: cfg.Artifacts}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	data, err := inv.IdentityJSON()
	if err == nil {
		err = os.WriteFile(opts["--out"], append(data, '\n'), 0600)
	}
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	_, _ = fmt.Fprintln(stdout, inv.IdentityHash())
	return 0
}
