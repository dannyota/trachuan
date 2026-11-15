package main

import (
	"context"
	"fmt"
	"os"

	"danny.vn/trachuan/apps/corpus/internal/testdb"
)

func main() {
	if err := testdb.Prune(context.Background(), os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
