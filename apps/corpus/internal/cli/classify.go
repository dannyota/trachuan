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
)

func parseScreenOptions(cc, rel string, args []string) (classify.Options, error) {
	o := classify.Options{Country: cc, Release: rel}
	if _, err := build.DatabaseName(cc, rel); err != nil {
		return o, err
	}
	opts, err := options(args, map[string]bool{"--select": true, "--documents": true, "--seed": true, "--recipe": true, "--out": true, "--budget": true, "--budget-usd": true, "--dry-run": false})
	if err != nil {
		return o, err
	}
	if selection, selected := opts["--select"]; selected {
		_, documents := opts["--documents"]
		_, seed := opts["--seed"]
		if documents || seed {
			return o, fmt.Errorf("--select is mutually exclusive with --documents and --seed")
		}
		var ok bool
		o.LabelsFile, ok = strings.CutPrefix(selection, "labels:")
		if !ok || o.LabelsFile == "" {
			return o, fmt.Errorf("--select requires labels:FILE")
		}
	} else {
		o.Documents, err = strconv.Atoi(opts["--documents"])
		if err != nil || o.Documents < 1 || o.Documents > 400 {
			return o, fmt.Errorf("--documents must be between 1 and 400")
		}
		o.Seed, err = strconv.ParseInt(opts["--seed"], 10, 64)
		if err != nil {
			return o, fmt.Errorf("--seed is required and must be an integer")
		}
	}
	o.Recipe, err = classify.NewRecipe(opts["--recipe"], strings.Repeat("0", 64))
	if err != nil {
		return o, err
	}
	o.Out = opts["--out"]
	if o.Out == "" {
		return o, fmt.Errorf("--out is required")
	}
	o.Budget = opts["--budget"]
	if o.Budget != "pilot" || opts["--budget-usd"] != "15" {
		return o, fmt.Errorf("screening requires --budget pilot --budget-usd 15")
	}
	o.BudgetMicroUSD = 15000000
	o.DryRun = opts["--dry-run"] == "true"
	return o, nil
}

func runClassify(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		switch args[0] {
		case "articles":
			return runClassifyArticles(ctx, args[1:], stdout, stderr)
		case "score":
			return runClassifyScore(ctx, args[1:], stdout, stderr)
		}
	}
	if len(args) < 3 || args[0] != "screen" {
		_, _ = fmt.Fprintln(stderr, "usage: corpus classify screen <cc> <rel> (--select labels:FILE | --documents N --seed S) --recipe MODEL --out DIR --budget pilot --budget-usd 15 [--dry-run]")
		return 2
	}
	o, err := parseScreenOptions(args[1], args[2], args[3:])
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 2
	}
	target, err := prepareScreen(ctx, o)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	defer target.close()
	o.RunID = target.runID
	o.Recipe.TaxonomyHash = target.taxonomyHash
	o.Selection = target.selection
	summary, err := target.runner.Screen(ctx, o, target.documents, target.families)
	if encodeErr := json.NewEncoder(stdout).Encode(summary); encodeErr != nil {
		_, _ = fmt.Fprintln(stderr, "write screening summary:", encodeErr)
		return 1
	}
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	if !summary.Admitted {
		return 1
	}
	return 0
}
