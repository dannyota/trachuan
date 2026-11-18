package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/config"
	"danny.vn/trachuan/apps/corpus/internal/stage/build"
	"danny.vn/trachuan/apps/corpus/internal/stage/evidence"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

type findingsLoader func(context.Context, string, string) (evidence.File, error)

func runFindings(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	return runFindingsWith(ctx, args, stdout, stderr, loadFindings)
}

func runFindingsWith(ctx context.Context, args []string, stdout, stderr io.Writer, load findingsLoader) int {
	usage := func() int {
		_, _ = fmt.Fprintln(stderr, "usage: corpus findings list <cc> <rel> | corpus findings show <cc> <rel> <key>")
		return 2
	}
	if len(args) < 3 {
		return usage()
	}
	action, cc, rel := args[0], args[1], args[2]
	if (action != "list" || len(args) != 3) && (action != "show" || len(args) != 4) {
		return usage()
	}
	if _, err := build.DatabaseName(cc, rel); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 2
	}
	file, err := load(ctx, cc, rel)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	if action == "show" {
		return showFinding(file, args[3], stdout, stderr)
	}
	table := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(table, "KEY\tDETECTOR\tSTATUS\tCARRIED\tFIRST_REL")
	for _, f := range file.Findings {
		_, _ = fmt.Fprintf(table, "%s\t%s\t%s\t%t\t%s\n", f.Key, f.Detector, f.Status, f.Carried, f.FirstRel)
	}
	if err = table.Flush(); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "findings %d new %d gone %d carried %d unmatched %d\n", len(file.Findings), len(file.New), len(file.Gone), len(file.Carried), len(file.Unmatched))
	return 0
}

func showFinding(file evidence.File, key string, stdout, stderr io.Writer) int {
	for _, f := range file.Findings {
		if f.Key != key {
			continue
		}
		data, err := json.MarshalIndent(f, "", "  ")
		if err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return 1
		}
		_, _ = fmt.Fprintf(stdout, "%s\n", data)
		return 0
	}
	_, _ = fmt.Fprintf(stderr, "finding %s not found\n", key)
	return 1
}

func loadFindings(ctx context.Context, cc, rel string) (evidence.File, error) {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return evidence.File{}, fmt.Errorf("load configuration: %w", err)
	}
	poolConfig, err := releaseRowPoolConfig(cfg.DatabaseURL, "")
	if err != nil {
		return evidence.File{}, err
	}
	db, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return evidence.File{}, fmt.Errorf("open workshop failed")
	}
	defer db.Close()
	return evidence.LoadFindings(ctx, store.New(db), artifact.Store{Root: cfg.Artifacts}, cc, rel)
}
