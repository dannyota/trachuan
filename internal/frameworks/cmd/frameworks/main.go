package main

import (
	"fmt"
	"os"

	"danny.vn/trachuan/internal/frameworks/command"
)

func main() {
	if err := command.Run(os.Args[1:], os.Stdout); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
