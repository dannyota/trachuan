package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"slices"
	"text/tabwriter"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/config"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/build"
	"danny.vn/trachuan/apps/corpus/internal/stage/evidence"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

type detectorCounter func(context.Context, string, string) ([]evidence.Count, error)

func runDetectorsCount(ctx context.Context, args []string, stdout, stderr io.Writer, load detectorCounter) int {
	if len(args) != 2 {
		_, _ = fmt.Fprintln(stderr, "usage: corpus detectors count <cc> <rel>")
		return 2
	}
	if _, err := build.DatabaseName(args[0], args[1]); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 2
	}
	counts, err := load(ctx, args[0], args[1])
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	table := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(table, "DETECTOR\tSEVERITY\tFINDINGS\tOBSERVED\tNOT_OBSERVABLE")
	code := 0
	for _, c := range counts {
		if c.NotRun != "" {
			code = 1
			_, _ = fmt.Fprintf(table, "%s\t%s\tnot run: %s\t\t\n", c.Detector, c.Severity, c.NotRun)
			continue
		}
		_, _ = fmt.Fprintf(table, "%s\t%s\t%d\t%d\t%d\n", c.Detector, c.Severity, c.Findings, c.Observed, c.NotObservable)
		classes := make([]string, 0, len(c.Classes))
		for class := range c.Classes {
			classes = append(classes, class)
		}
		slices.Sort(classes)
		for _, class := range classes {
			_, _ = fmt.Fprintf(table, "  %s %d\t\t\t\t\n", class, c.Classes[class])
		}
	}
	if err = table.Flush(); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	return code
}

func loadDetectorCounts(ctx context.Context, cc, rel string) ([]evidence.Count, error) {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return nil, fmt.Errorf("load configuration: %w", err)
	}
	name, err := build.DatabaseName(cc, rel)
	if err != nil {
		return nil, err
	}
	workshopConfig, err := releaseRowPoolConfig(cfg.DatabaseURL, "")
	if err != nil {
		return nil, err
	}
	workshop, err := pgxpool.NewWithConfig(ctx, workshopConfig)
	if err != nil {
		return nil, fmt.Errorf("open workshop failed")
	}
	defer workshop.Close()
	releaseConfig, err := releaseRowPoolConfig(cfg.BuilderDatabaseURL, name)
	if err != nil {
		return nil, err
	}
	release, err := pgxpool.NewWithConfig(ctx, releaseConfig)
	if err != nil {
		return nil, fmt.Errorf("open release database failed")
	}
	defer release.Close()
	q := store.New(workshop)
	row, err := q.GetRun(ctx, store.GetRunParams{Country: cc, Rel: rel})
	if err != nil {
		return nil, fmt.Errorf("read run %s/%s: %w", cc, rel, err)
	}
	var snapshot stage.Snapshot
	if err = json.Unmarshal(row.Snapshot, &snapshot); err != nil {
		return nil, fmt.Errorf("decode run snapshot: %w", err)
	}
	run := stage.Run{ID: row.ID, Country: row.Country, Rel: row.Rel, Generation: int(row.Generation), Snapshot: snapshot}
	env := stage.Env{DB: workshop, Artifacts: artifact.Store{Root: cfg.Artifacts}}
	return evidence.CountFindings(ctx, evidence.CountInputs(ctx, q, env, run, release), evidence.CrossSignal()), nil
}
