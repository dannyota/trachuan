package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/ocrcheck"
)

func parseCheckOptions(args []string, review bool) (ocrcheck.Options, error) {
	allowed := map[string]bool{"--model": true, "--effort": true, "--check-version": true}
	if !review {
		for k, v := range map[string]bool{"--entries": true, "--in-flight": true, "--limit-docs": true, "--max-usd": true, "--batch": false, "--dry-run": false, "--reconcile": false, "--retry-failed": false, "--abandon": true} {
			allowed[k] = v
		}
	}
	flags, err := options(args, allowed)
	if err != nil {
		return ocrcheck.Options{}, err
	}
	o := ocrcheck.Options{Abandon: flags["--abandon"], Model: "claude-sonnet-5-5", Effort: "medium", Batch: flags["--batch"] != "", DryRun: flags["--dry-run"] != "", Reconcile: flags["--reconcile"] != "", RetryFailed: flags["--retry-failed"] != ""}
	if value, ok := flags["--check-version"]; ok {
		o.CheckVersion, err = strconv.Atoi(value)
		if err != nil || o.CheckVersion == 0 {
			return o, fmt.Errorf("--check-version must be 2, 3 or 4")
		}
	}
	if err = o.ValidateVersion(); err != nil {
		return o, err
	}
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
	if value, ok := flags["--entries"]; ok {
		seen := map[int64]bool{}
		for _, part := range strings.Split(value, ",") {
			id, parseErr := strconv.ParseInt(part, 10, 64)
			if parseErr != nil || id < 1 || seen[id] {
				return o, fmt.Errorf("--entries requires distinct positive comma-separated OCR text ids")
			}
			seen[id] = true
			o.Entries = append(o.Entries, id)
		}
	}
	if value, ok := flags["--limit-docs"]; ok {
		o.LimitDocs, err = strconv.Atoi(value)
		if err != nil || o.LimitDocs < 0 {
			return o, fmt.Errorf("--limit-docs must be nonnegative")
		}
	}
	if value, ok := flags["--in-flight"]; ok {
		o.InFlight, err = strconv.Atoi(value)
		if err != nil || o.InFlight < 1 || o.InFlight > 8 || o.CheckVersion < 3 {
			return o, fmt.Errorf("--in-flight requires check version 3 or 4 and a value from 1 to 8")
		}
	}
	if o.Reconcile && o.DryRun {
		return o, fmt.Errorf("--reconcile and --dry-run cannot be combined")
	}
	if err = o.Validate(); err != nil {
		return o, err
	}
	return o, nil
}

func runOCRCheck(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) < 2 {
		_, _ = fmt.Fprintln(stderr, "usage: corpus ocr check <cc> <rel> --max-usd USD [--model MODEL] [--effort EFFORT] [--check-version 2|3|4] [--limit-docs N] [--entries ID[,ID...]] [--in-flight N] [--batch] [--dry-run] [--reconcile] [--retry-failed] [--abandon SUBMISSION_ID]")
		return 2
	}
	o, err := parseCheckOptions(args[2:], false)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 2
	}
	root := os.Getenv("CORPUS_OCR_STORE")
	if root == "" {
		_, _ = fmt.Fprintln(stderr, "CORPUS_OCR_STORE is required")
		return 1
	}
	if !o.DryRun {
		unlock, lockErr := lockOCRStore(root)
		if lockErr != nil {
			_, _ = fmt.Fprintln(stderr, lockErr)
			return 1
		}
		defer unlock()
	}
	env, err := openCheckRun(ctx, args[0], args[1])
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	defer env.db.Close()
	var client ocrcheck.Client
	if o.Abandon == "" {
		client, err = ocrcheck.NewSDKClient()
		if err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return 1
		}
	}
	runner := ocrcheck.Runner{Client: client, Repository: ocrcheck.NewDatabase(env.db, env.root), LoadDocuments: func(ctx context.Context) ([]ocrcheck.Document, error) {
		return env.checkDocuments(ctx, o.CheckerID(), o.CheckVersion >= 3)
	}}
	summary, err := runner.Run(ctx, nil, o)
	_, _ = fmt.Fprintf(stdout, "checker: %s\ncomplete: %d\nfailed: %d\nskipped_retry_limit: %d\nsubmitted: %d\npending: %d\nattached: %d\nabandoned: %d\nspend USD: %.6f\nestimated documents: %d\nworst-case estimate USD: %.6f\n", o.CheckerID(), summary.Complete, summary.Failed, summary.SkippedRetryLimit, summary.Submitted, summary.Pending, summary.Attached, summary.Abandoned, float64(summary.CostMicroUSD)/1e6, summary.Estimated, float64(summary.EstimateMicroUSD)/1e6)
	printCheckDocuments(stdout, summary.Documents)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		if errors.Is(err, ocrcheck.ErrBudget) {
			return 2
		}
		return 1
	}
	return 0
}

func printCheckDocuments(out io.Writer, documents []ocrcheck.DocumentSummary) {
	for _, doc := range documents {
		_, _ = fmt.Fprintf(out, "ocr_text_id=%d pages=%d input_tokens=%d output_tokens=%d cost_usd=%.6f outcome=%s\n", doc.EntryID, doc.Pages, doc.InputTokens, doc.OutputTokens, float64(doc.CostMicroUSD)/1e6, doc.Outcome)
	}
}
