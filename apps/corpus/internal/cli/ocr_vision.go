package cli

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/ocrstore"
	"danny.vn/trachuan/apps/corpus/internal/ocrvision"
)

func parseVisionFlags(args []string) (ocrvision.Options, string, error) {
	flags, err := options(args, map[string]bool{"--max-pages": true, "--limit-docs": true, "--pdf": true, "--dry-run": false})
	var o ocrvision.Options
	if err != nil {
		return o, "", fmt.Errorf("invalid Vision options")
	}
	o.MaxPages, err = strconv.Atoi(flags["--max-pages"])
	if err != nil || o.MaxPages < 1 {
		return o, "", fmt.Errorf("a positive --max-pages is required")
	}
	if value, ok := flags["--limit-docs"]; ok {
		o.LimitDocs, err = strconv.Atoi(value)
		if err != nil || o.LimitDocs < 0 {
			return o, "", fmt.Errorf("--limit-docs must be nonnegative")
		}
	}
	o.DryRun = flags["--dry-run"] != ""
	pdf := flags["--pdf"]
	if _, ok := flags["--pdf"]; ok {
		if len(pdf) != 64 || strings.ToLower(pdf) != pdf {
			return o, "", fmt.Errorf("--pdf must be a lowercase SHA-256")
		}
		if _, err = hex.DecodeString(pdf); err != nil {
			return o, "", fmt.Errorf("--pdf must be a lowercase SHA-256")
		}
	}
	return o, pdf, nil
}

func runOCRVision(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) < 2 {
		_, _ = fmt.Fprintln(stderr, "usage: corpus ocr vision <cc> <rel> --max-pages N [--limit-docs N] [--pdf SHA256] [--dry-run]")
		return 2
	}
	o, pdf, err := parseVisionFlags(args[2:])
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 2
	}
	var client *ocrvision.Client
	if !o.DryRun {
		client, err = ocrvision.NewAuthenticated(ctx)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return 1
		}
		unlock, lockErr := lockOCRStore(os.Getenv("CORPUS_OCR_STORE"))
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
	docs, err := env.visionDocuments(ctx, pdf)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	runner := ocrvision.Runner{Client: client, Store: ocrstore.Open(env.root, env.db)}
	if _, err = runner.Run(ctx, docs, o, stdout); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}
