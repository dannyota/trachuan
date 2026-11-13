//go:build integration

package main

import (
	"context"
	"fmt"
	"os"

	"danny.vn/trachuan/apps/server/cmd/e2efixture/internal/fixture"
)

func main() {
	ctx := context.Background()
	if len(os.Args) == 2 && os.Args[1] == "create" {
		create := fixture.Create
		if os.Getenv("E2E_RECIPE3") == "1" {
			create = fixture.CreateRecipe3
		}
		name, key, err := create(ctx)
		if err != nil {
			fmt.Fprintln(os.Stderr, "fixture create failed:", err)
			if name != "" {
				fmt.Fprintln(os.Stderr, "fixture identity:", name, key)
			}
			os.Exit(1)
		}
		fmt.Println(name, key)
		return
	}
	if len(os.Args) == 4 && os.Args[1] == "drop" {
		if err := fixture.Drop(ctx, os.Args[2], os.Args[3]); err != nil {
			fmt.Fprintln(os.Stderr, "fixture drop failed:", err)
			os.Exit(1)
		}
		return
	}
	fmt.Fprintln(os.Stderr, "usage: e2efixture create | drop DATABASE RUN_KEY")
	os.Exit(1)
}
