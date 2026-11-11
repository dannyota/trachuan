package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"

	"danny.vn/trachuan/apps/corpus/internal/ocrcheck"
)

func parseCheckOptions(args []string, review bool) (ocrcheck.Options, error) {
	allowed := map[string]bool{"--model": true, "--effort": true}
	if !review {
		for k, v := range map[string]bool{"--limit-docs": true, "--max-usd": true, "--batch": false, "--dry-run": false, "--reconcile": false} {
			allowed[k] = v
		}
	}
	flags, err := options(args, allowed)
	if err != nil {
		return ocrcheck.Options{}, err
	}
	o := ocrcheck.Options{Model: "claude-sonnet-5-5", Effort: "medium", Batch: flags["--batch"] != "", DryRun: flags["--dry-run"] != "", Reconcile: flags["--reconcile"] != ""}
	if flags["--model"] != "" {
		o.Model = flags["--model"]
	}
	if flags["--effort"] != "" {
		o.Effort = flags["--effort"]
	}
	if o.Model != "claude-sonnet-5-5" && o.Model != "claude-opus-5-5" {
		return o, fmt.Errorf("--model must be claude-sonnet-5-5 or claude-opus-5-5")
	}
	switch o.Effort {
	case "low", "medium", "high", "xhigh", "max":
	default:
		return o, fmt.Errorf("unsupported --effort")
	}
	if review {
		return o, nil
	}
	o.MaxUSD, err = strconv.ParseFloat(flags["--max-usd"], 64)
	if err != nil || o.MaxUSD <= 0 || math.IsNaN(o.MaxUSD) || math.IsInf(o.MaxUSD, 0) || o.MaxUSD > 1e9 {
		return o, fmt.Errorf("a positive finite --max-usd is required")
	}
	if value, ok := flags["--limit-docs"]; ok {
		o.LimitDocs, err = strconv.Atoi(value)
		if err != nil || o.LimitDocs < 0 {
			return o, fmt.Errorf("--limit-docs must be nonnegative")
		}
	}
	if o.Reconcile && o.DryRun {
		return o, fmt.Errorf("--reconcile and --dry-run cannot be combined")
	}
	return o, nil
}

func runOCRCheck(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) < 2 {
		_, _ = fmt.Fprintln(stderr, "usage: corpus ocr check <cc> <rel> --max-usd USD [--model MODEL] [--effort EFFORT] [--limit-docs N] [--batch] [--dry-run] [--reconcile]")
		return 2
	}
	o, err := parseCheckOptions(args[2:], false)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 2
	}
	env, err := openCheckRun(ctx, args[0], args[1])
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	defer env.db.Close()
	if !o.DryRun {
		unlock, lockErr := lockOCRStore(env.root)
		if lockErr != nil {
			_, _ = fmt.Fprintln(stderr, lockErr)
			return 1
		}
		defer unlock()
	}
	var docs []ocrcheck.Document
	if !o.Reconcile {
		docs, err = env.documents(ctx, o.CheckerID())
		if err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return 1
		}
	}
	client, err := ocrcheck.NewSDKClient()
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	runner := ocrcheck.Runner{Client: client, Repository: ocrcheck.NewDatabase(env.db, env.root)}
	summary, err := runner.Run(ctx, docs, o)
	_, _ = fmt.Fprintf(stdout, "checker: %s\ncomplete: %d\nfailed: %d\nsubmitted: %d\npending: %d\nattached: %d\nabandoned: %d\nspend USD: %.6f\nestimated documents: %d\nworst-case estimate USD: %.6f\n", o.CheckerID(), summary.Complete, summary.Failed, summary.Submitted, summary.Pending, summary.Attached, summary.Abandoned, float64(summary.CostMicroUSD)/1e6, summary.Estimated, float64(summary.EstimateMicroUSD)/1e6)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		if errors.Is(err, ocrcheck.ErrBudget) {
			return 2
		}
		return 1
	}
	return 0
}
