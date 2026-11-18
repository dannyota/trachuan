package cli

import (
	"context"
	"errors"
	"fmt"
	"io"

	"danny.vn/trachuan/apps/corpus/internal/knowledge"
	"danny.vn/trachuan/apps/corpus/internal/stage/build"
	"danny.vn/trachuan/internal/corpusdb"
	"github.com/jackc/pgx/v5/pgxpool"
)

type knowledgeRelease interface {
	knowledge.Reader
	HasRelease(ctx context.Context, country, rel string, schemaVersion int) (bool, error)
}

func runKnowledge(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	switch {
	case len(args) == 2 && args[0] == "check":
		return knowledgeCheck(ctx, args[1], "", stdout, stderr)
	case len(args) == 3 && args[0] == "check":
		return knowledgeCheck(ctx, args[1], args[2], stdout, stderr)
	case len(args) == 5 && args[0] == "depends":
		return knowledgeDepends(ctx, args[1:], stdout, stderr)
	}
	return knowledgeUsage(stderr)
}

func knowledgeUsage(stderr io.Writer) int {
	_, _ = fmt.Fprintln(stderr, "usage: corpus knowledge check <cc> [<rel>] | corpus knowledge depends <cc> <rel> <key> <anchor>")
	return 2
}

func knowledgeCheck(ctx context.Context, cc, rel string, stdout, stderr io.Writer) int {
	path, err := knowledge.SourcePath(cc)
	if err != nil {
		return knowledgeUsage(stderr)
	}
	database := ""
	if rel != "" {
		if database, err = build.DatabaseName(cc, rel); err != nil {
			return knowledgeUsage(stderr)
		}
	}
	file, _, err := knowledge.ParseFile(cc, path)
	if err != nil {
		printKnowledgeError(stderr, err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "validated %d entries: %d decisions, %d effects and relations\n", file.Len(), len(file.Decisions), len(file.Claims))
	if rel == "" {
		return 0
	}
	db, err := openKnowledgeRelease(ctx, database)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	defer db.Close()
	return checkKnowledgeRelease(ctx, knowledge.SQLReader{Q: db}, file, cc, rel, stdout, stderr)
}

func openKnowledgeRelease(ctx context.Context, database string) (*pgxpool.Pool, error) {
	return headingPool(ctx, "CORPUS_BUILDER_DATABASE_URL", database)
}

func requireKnowledgeRelease(ctx context.Context, db knowledgeRelease, cc, rel string) error {
	found, err := db.HasRelease(ctx, cc, rel, corpusdb.SchemaVersion)
	if err != nil {
		return fmt.Errorf("read release %s/%s: %w", cc, rel, err)
	}
	if !found {
		return fmt.Errorf("release %s/%s at schema %d is not in this database", cc, rel, corpusdb.SchemaVersion)
	}
	return nil
}

func checkKnowledgeRelease(ctx context.Context, db knowledgeRelease, file knowledge.File, cc, rel string, stdout, stderr io.Writer) int {
	if err := requireKnowledgeRelease(ctx, db, cc, rel); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	report, err := knowledge.Check(ctx, db, file)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	if err = printKnowledgeReport(stdout, report); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	if err = report.Err(); err != nil {
		printKnowledgeError(stderr, err)
		return 1
	}
	return 0
}

func knowledgeDepends(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	cc, rel, key, anchor := args[0], args[1], args[2], args[3]
	database, err := build.DatabaseName(cc, rel)
	if err != nil {
		return knowledgeUsage(stderr)
	}
	db, err := openKnowledgeRelease(ctx, database)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	defer db.Close()
	return printKnowledgeDepends(ctx, knowledge.SQLReader{Q: db}, cc, rel, key, anchor, stdout, stderr)
}

func printKnowledgeDepends(ctx context.Context, db knowledgeRelease, cc, rel, key, anchor string, stdout, stderr io.Writer) int {
	if err := requireKnowledgeRelease(ctx, db, cc, rel); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	depends, err := knowledge.Depends(ctx, db, key, anchor)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	data, err := depends.YAML()
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	if _, err = stdout.Write(data); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

func printKnowledgeError(stderr io.Writer, err error) {
	var invalid *knowledge.ValidationError
	if !errors.As(err, &invalid) {
		_, _ = fmt.Fprintln(stderr, err)
		return
	}
	for _, problem := range invalid.Problems {
		_, _ = fmt.Fprintln(stderr, problem)
	}
}

func printKnowledgeReport(w io.Writer, report knowledge.Report) error {
	counts := map[string]int{}
	for _, r := range report.Records {
		counts[r.Outcome]++
		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", r.ID, r.Kind, r.TargetKey, r.Outcome, r.Reason); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintf(w, "applied %d, redundant %d, stale %d\n", counts[knowledge.OutcomeApplied], counts[knowledge.OutcomeRedundant], counts[knowledge.OutcomeStale])
	return err
}
