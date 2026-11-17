package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/classify"
	"danny.vn/trachuan/apps/corpus/internal/stage/build"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"danny.vn/trachuan/apps/corpus/internal/taxonomy"
)

type articleOptions struct {
	Options          classify.Options
	Families, Labels string
	MaxTokens        int64
	Step             int
}

func withArticleMaxTokens(r classify.Recipe, maxTokens int64) (classify.Recipe, error) {
	if maxTokens == 0 {
		return r, nil
	}
	if r.Article == nil {
		return r, fmt.Errorf("max tokens apply to an article recipe")
	}
	article := *r.Article
	article.MaxTokens = maxTokens
	r.Article = &article
	return r, r.Validate()
}

func parseArticleOptions(cc, rel string, args []string) (articleOptions, error) {
	var o articleOptions
	if _, err := build.DatabaseName(cc, rel); err != nil {
		return o, err
	}
	flags, err := options(args, map[string]bool{"--families": true, "--select": true, "--recipe": true, "--out": true, "--budget": true, "--budget-usd": true, "--max-tokens": true, "--dry-run": false, "--step": true, "--step-1": true})
	if err != nil {
		return o, err
	}
	if value, set := flags["--max-tokens"]; set {
		o.MaxTokens, err = strconv.ParseInt(value, 10, 64)
		if err != nil || o.MaxTokens < 1 || o.MaxTokens > 32000 {
			return o, fmt.Errorf("--max-tokens must be an integer from 1 to 32000")
		}
	}
	if value, set := flags["--step"]; set {
		if value != "1" && value != "2" {
			return o, fmt.Errorf("--step must be 1 or 2")
		}
		o.Step, _ = strconv.Atoi(value)
		if flags["--recipe"] != "claude-haiku-5-5" {
			return o, fmt.Errorf("two-step trial requires Haiku")
		}
	}
	if (o.Step == 2) != (flags["--step-1"] != "") {
		return o, fmt.Errorf("--step-1 FILE is required only with --step 2")
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
	o = articleOptions{Options: classify.Options{Country: cc, Release: rel, Recipe: recipe, Out: flags["--out"], Budget: "pilot", BudgetMicroUSD: 15000000, DryRun: flags["--dry-run"] == "true", Step1Artifact: flags["--step-1"]}, Families: flags["--families"], Labels: selection, MaxTokens: o.MaxTokens, Step: o.Step}
	return o, nil
}

func runClassifyArticles(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	return runClassifyArticlesWith(ctx, args, stdout, stderr, loadClassificationEvidence)
}

func runClassifyArticlesWith(ctx context.Context, args []string, stdout, stderr io.Writer, load func(context.Context, string, string, taxonomy.Labels) (classificationEvidence, error)) int {
	fail := func(code int, err error) int { _, _ = fmt.Fprintln(stderr, err); return code }
	if len(args) < 2 {
		return fail(2, fmt.Errorf("usage: corpus classify articles <cc> <rel> --families FILE --select labels:FILE --recipe MODEL --out DIR --budget pilot --budget-usd 15 [--max-tokens N] [--step 1|2] [--step-1 FILE] [--dry-run]"))
	}
	o, err := parseArticleOptions(args[0], args[1], args[2:])
	if err != nil {
		return fail(2, err)
	}
	labels, labelsHash, err := classify.ReadLabels(o.Labels)
	if err != nil {
		return fail(1, err)
	}
	if n := len(labels.Articles); n > 300 {
		return fail(1, fmt.Errorf("article pilot takes at most 300 labeled articles, got %d", n))
	}
	evidence, err := load(ctx, args[0], args[1], labels)
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
	if err = classify.ValidateArticleFamilies(evidence.articles, families); err != nil {
		return fail(1, err)
	}
	o.Options.Recipe, err = classify.NewArticleRecipe(families.Recipe, hash)
	if err == nil && o.Step != 0 {
		var step1Hash string
		if o.Step == 2 {
			_, step1Hash, err = classify.ReadStep1Artifact(o.Options.Step1Artifact, evidence.taxonomyHash, hash)
		}
		if err == nil {
			o.Options.Recipe, err = classify.NewTwoStepArticleRecipe(families.Recipe, hash, o.Step, step1Hash)
		}
	}
	if err == nil {
		o.Options.Recipe, err = withArticleMaxTokens(o.Options.Recipe, o.MaxTokens)
	}
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
