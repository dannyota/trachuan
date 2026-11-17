//go:build integration

package main

import (
	"context"
	"fmt"
	"os"

	"danny.vn/trachuan/apps/corpus/cmd/e2esummary/internal/summary"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: e2esummary DATABASE")
		os.Exit(1)
	}
	if err := summary.Load(context.Background(), os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, "summary fixture failed:", err)
		os.Exit(1)
	}
}
