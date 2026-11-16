package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/stage/build"
	"github.com/jackc/pgx/v5/pgxpool"
)

func runHeadingScan(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) != 2 {
		_, _ = fmt.Fprintln(stderr, "usage: corpus heading-scan <cc> <rel>")
		return 2
	}
	name, err := build.DatabaseName(args[0], args[1])
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 2
	}
	if err = headingScan(ctx, args[0], args[1], name, stdout); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

func headingPool(ctx context.Context, variable, database string) (*pgxpool.Pool, error) {
	url := os.Getenv(variable)
	if url == "" {
		return nil, fmt.Errorf("%s is required", variable)
	}
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("invalid %s", variable)
	}
	if database != "" {
		cfg.ConnConfig.Database = database
	}
	cfg.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("open heading-check database failed")
	}
	return db, nil
}

func headingScan(ctx context.Context, cc, rel, name string, stdout io.Writer) error {
	db, err := headingPool(ctx, "CORPUS_BUILDER_DATABASE_URL", name)
	if err != nil {
		return err
	}
	defer db.Close()
	var input, recipe string
	if err = db.QueryRow(ctx, `SELECT build_input_hash,build_recipe_hash FROM release WHERE singleton AND country=$1 AND rel=$2 AND schema_version=8`, cc, rel).Scan(&input, &recipe); err != nil {
		return fmt.Errorf("read heading scan release failed")
	}
	data, err := build.HeadingScan(ctx, db, cc, rel)
	if err != nil {
		return err
	}
	workshop, err := headingPool(ctx, "CORPUS_DATABASE_URL", "")
	if err != nil {
		return err
	}
	defer workshop.Close()
	var path, expected string
	err = workshop.QueryRow(ctx, `SELECT a.path,a.sha256 FROM run r JOIN run_item i ON i.run_id=r.id JOIN artifact a ON a.id=i.artifact_id WHERE r.country=$1 AND r.rel=$2 AND i.stage='build' AND i.item_key=$1||'/'||$2 AND i.state='done' AND i.input_hash=$3 AND i.recipe_hash=$4 AND a.input_hash=$3 AND a.recipe_hash=$4`, cc, rel, input, recipe).Scan(&path, &expected)
	if err != nil {
		return fmt.Errorf("read heading scan build record failed")
	}
	root := os.Getenv("CORPUS_ARTIFACTS")
	if root == "" {
		root = "work/artifacts"
	}
	dir := (artifact.Store{Root: root}).Abs(path)
	hash, err := artifact.Verify(dir)
	if err != nil {
		return err
	}
	if hash != expected {
		return fmt.Errorf("heading scan build artifact hash mismatch")
	}
	scanHash, err := build.WriteHeadingScan(dir, data)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "%s  %s\n", scanHash, filepath.Join(dir, "heading-scan.json"))
	return err
}

func runHeadingExclusions(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) != 3 || args[0] != "check" {
		_, _ = fmt.Fprintln(stderr, "usage: corpus heading-exclusions check <cc> <rel>")
		return 2
	}
	name, err := build.DatabaseName(args[1], args[2])
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 2
	}
	data, err := os.ReadFile("sources/" + args[1] + "-heading-exclusions.yaml")
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "read heading exclusions failed")
		return 1
	}
	entries, err := source.ParseHeadingExclusions(data)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	db, err := headingPool(ctx, "CORPUS_BUILDER_DATABASE_URL", name)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	defer db.Close()
	if err = build.CheckHeadingExclusions(ctx, db, entries); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "validated %d heading exclusions\n", len(entries))
	return 0
}

func runHeadingReviews(_ context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) != 3 || args[0] != "check" {
		_, _ = fmt.Fprintln(stderr, "usage: corpus heading-reviews check <cc> <rel>")
		return 2
	}
	if _, err := build.DatabaseName(args[1], args[2]); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 2
	}
	data, err := os.ReadFile("sources/" + args[1] + "-heading-reviews.yaml")
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "read heading reviews failed")
		return 1
	}
	entries, err := source.ParseHeadingReviews(data)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "validated %d heading reviews\n", len(entries))
	return 0
}
