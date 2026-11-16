package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/classify"
	"danny.vn/trachuan/apps/corpus/internal/stage/build"
	"danny.vn/trachuan/apps/corpus/internal/store"
)

type articleOptions struct {
	Options          classify.Options
	Families, Labels string
}

func parseArticleOptions(cc, rel string, args []string) (articleOptions, error) {
	var o articleOptions
	if _, err := build.DatabaseName(cc, rel); err != nil {
		return o, err
	}
	flags, err := options(args, map[string]bool{"--families": true, "--select": true, "--recipe": true, "--out": true, "--budget": true, "--budget-usd": true, "--dry-run": false})
	if err != nil {
		return o, err
	}
	selection, ok := strings.CutPrefix(flags["--select"], "labels:")
	if !ok || selection == "" || flags["--families"] == "" || flags["--out"] == "" {
		return o, fmt.Errorf("articles requires --families FILE --select labels:FILE --out DIR")
	}
	if flags["--budget"] != "pilot" || flags["--budget-usd"] != "15" {
		return o, fmt.Errorf("articles requires --budget pilot --budget-usd 15")
	}
	recipe, err := classify.NewRecipe(flags["--recipe"], strings.Repeat("0", 64))
	if err != nil {
		return o, err
	}
	o = articleOptions{Options: classify.Options{Country: cc, Release: rel, Recipe: recipe, Out: flags["--out"], Budget: "pilot", BudgetMicroUSD: 15000000, DryRun: flags["--dry-run"] == "true"}, Families: flags["--families"], Labels: selection}
	return o, nil
}

func runClassifyArticles(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fail := func(code int, err error) int { _, _ = fmt.Fprintln(stderr, err); return code }
	if len(args) < 2 {
		return fail(2, fmt.Errorf("usage: corpus classify articles <cc> <rel> --families FILE --select labels:FILE --recipe MODEL --out DIR --budget pilot --budget-usd 15 [--dry-run]"))
	}
	o, err := parseArticleOptions(args[0], args[1], args[2:])
	if err != nil {
		return fail(2, err)
	}
	labels, labelsHash, err := classify.ReadLabels(o.Labels)
	if err != nil {
		return fail(1, err)
	}
	if len(labels.Articles) != 300 {
		return fail(1, fmt.Errorf("article pilot requires exactly 300 labeled articles"))
	}
	evidence, err := loadClassificationEvidence(ctx, args[0], args[1], labels)
	if err != nil {
		return fail(1, err)
	}
	if err = classify.ValidateSelection(labels, evidence.articles, evidence.nodes); err != nil {
		return fail(1, err)
	}
	families, hash, err := classify.ReadFamilyArtifact(o.Families, evidence.taxonomyHash, evidence.nodes)
	if err != nil {
		return fail(1, err)
	}
	if families.Recipe.Model != o.Options.Recipe.Model {
		return fail(1, fmt.Errorf("article model must match the document recipe"))
	}
	o.Options.Recipe, err = classify.NewArticleRecipe(families.Recipe, hash)
	if err != nil {
		return fail(1, err)
	}
	runner, workshop, _, err := openSubmissionRunner(ctx, true)
	if err != nil {
		return fail(1, err)
	}
	defer workshop.Close()
	run, err := store.New(workshop).GetRun(ctx, store.GetRunParams{Country: args[0], Rel: args[1]})
	if err != nil {
		return fail(1, fmt.Errorf("read article classification run: %w", err))
	}
	o.Options.RunID = run.ID
	summary, err := runner.Articles(ctx, o.Options, labelsHash, evidence.articles, families, evidence.nodes)
	if encodeErr := json.NewEncoder(stdout).Encode(summary); encodeErr != nil {
		return fail(1, fmt.Errorf("write article summary: %w", encodeErr))
	}
	if err != nil {
		return fail(1, err)
	}
	if !summary.Admitted {
		return 1
	}
	return 0
}
