package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"danny.vn/trachuan/apps/corpus/internal/classify"
	"danny.vn/trachuan/apps/corpus/internal/stage/build"
	"danny.vn/trachuan/apps/corpus/internal/taxonomy"
)

type scoreOptions struct{ Labels, Families, Articles, Out string }

func parseScoreOptions(cc, rel string, args []string) (scoreOptions, error) {
	var o scoreOptions
	if _, err := build.DatabaseName(cc, rel); err != nil {
		return o, err
	}
	flags, err := options(args, map[string]bool{"--labels": true, "--families": true, "--articles": true, "--out": true})
	if err != nil {
		return o, err
	}
	o = scoreOptions{flags["--labels"], flags["--families"], flags["--articles"], flags["--out"]}
	if o.Labels == "" || o.Families == "" || o.Articles == "" || o.Out == "" {
		return o, fmt.Errorf("score requires --labels FILE --families FILE --articles FILE --out DIR")
	}
	return o, nil
}

func runClassifyScore(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	return runClassifyScoreWith(ctx, args, stdout, stderr, loadClassificationEvidence)
}

func runClassifyScoreWith(ctx context.Context, args []string, stdout, stderr io.Writer, load func(context.Context, string, string, taxonomy.Labels) (classificationEvidence, error)) int {
	fail := func(code int, err error) int { _, _ = fmt.Fprintln(stderr, err); return code }
	if len(args) < 2 {
		return fail(2, fmt.Errorf("usage: corpus classify score <cc> <rel> --labels FILE --families FILE --articles FILE --out DIR"))
	}
	o, err := parseScoreOptions(args[0], args[1], args[2:])
	if err != nil {
		return fail(2, err)
	}
	labels, labelsHash, err := classify.ReadLabels(o.Labels)
	if err != nil {
		return fail(1, err)
	}
	evidence, err := load(ctx, args[0], args[1], labels)
	if err != nil {
		return fail(1, err)
	}
	families, familiesHash, err := classify.ReadFamilyArtifactForScore(o.Families, evidence.taxonomyHash, evidence.nodes)
	if err != nil {
		return fail(1, err)
	}
	articles, err := classify.ReadArticleArtifactForScore(o.Articles, evidence.taxonomyHash, familiesHash)
	if err != nil {
		return fail(1, err)
	}
	if articles.Country != args[0] || articles.Release != args[1] || articles.LabelsHash != labelsHash || articles.Recipe.Article.SourceRecipeHash != families.RecipeHash {
		return fail(1, fmt.Errorf("article artifact differs from target release, labels or document recipe"))
	}
	report, scoreErr := classify.Score(labels, families, articles, evidence.articles, evidence.nodes)
	report.LabelsHash = labelsHash
	if scoreErr != nil && report.Status != "unavailable" {
		return fail(1, scoreErr)
	}
	if err = classify.WriteScore(o.Out, report); err != nil {
		return fail(1, err)
	}
	if err = json.NewEncoder(stdout).Encode(report); err != nil {
		return fail(1, fmt.Errorf("write classification scores: %w", err))
	}
	if scoreErr != nil {
		return fail(1, scoreErr)
	}
	return 0
}
