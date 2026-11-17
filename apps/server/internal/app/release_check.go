package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"danny.vn/trachuan/apps/server/internal/releasecheck"
	"danny.vn/trachuan/internal/embedding"
)

func runReleaseCheck(ctx context.Context, args []string, stdout io.Writer) error {
	kind, database, err := releaseCheckArguments(args)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	base := os.Getenv("SERVER_QUERY_EMBEDDER_URL")
	if base == "" {
		base = "http://127.0.0.1:8091"
	}
	client := embedding.NewClient(base, &http.Client{Timeout: 10 * time.Second})
	if kind == "frameworks" {
		var report any
		if database == "" {
			report, err = releasecheck.DescribeFrameworks(ctx, client)
		} else {
			report, err = releasecheck.CheckFrameworks(ctx, os.Getenv("SERVER_CORPUS_DATABASE_URL"), database, client)
		}
		if err != nil {
			return err
		}
		if err = json.NewEncoder(stdout).Encode(report); err != nil {
			return fmt.Errorf("write frameworks report: %w", err)
		}
		return nil
	}
	report, err := releasecheck.Describe(ctx, client)
	if err != nil {
		return fmt.Errorf("describe release compatibility: %w", err)
	}
	if database != "" {
		if err := releasecheck.Check(ctx, os.Getenv("SERVER_CORPUS_DATABASE_URL"), database, client); err != nil {
			return fmt.Errorf("check corpus release: %w", err)
		}
	}
	if err := json.NewEncoder(stdout).Encode(report); err != nil {
		return fmt.Errorf("write compatibility report: %w", err)
	}
	return nil
}

func releaseCheckArguments(args []string) (string, string, error) {
	kind, database := "", ""
	seen := map[string]bool{}
	for len(args) > 0 {
		if len(args) < 2 || seen[args[0]] {
			return "", "", errors.New("invalid release-check arguments")
		}
		seen[args[0]] = true
		switch args[0] {
		case "--kind":
			if args[1] != "frameworks" {
				return "", "", errors.New("invalid release-check arguments")
			}
			kind = args[1]
		case "--database":
			database = args[1]
		default:
			return "", "", errors.New("invalid release-check arguments")
		}
		args = args[2:]
	}
	if seen["--database"] && ((kind == "" && !releasecheck.ValidDatabase(database)) || (kind == "frameworks" && !releasecheck.ValidFrameworksDatabase(database))) {
		return "", "", errors.New("invalid release-check arguments")
	}
	return kind, database, nil
}
