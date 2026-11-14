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
	database := ""
	if len(args) != 0 {
		if len(args) != 2 || args[0] != "--database" || !releasecheck.ValidDatabase(args[1]) {
			return errors.New("invalid release-check arguments")
		}
		database = args[1]
	}
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	base := os.Getenv("SERVER_QUERY_EMBEDDER_URL")
	if base == "" {
		base = "http://127.0.0.1:8091"
	}
	client := embedding.NewClient(base, &http.Client{Timeout: 10 * time.Second})
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
