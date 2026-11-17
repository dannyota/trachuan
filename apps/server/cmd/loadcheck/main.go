package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"danny.vn/trachuan/apps/server/cmd/loadcheck/internal/loadrun"
	"danny.vn/trachuan/apps/server/cmd/loadcheck/internal/observe"
	"danny.vn/trachuan/apps/server/cmd/loadcheck/internal/queryfile"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var err error
	args := os.Args[1:]
	if len(args) == 0 {
		err = errors.New("usage: loadcheck run|queries|sample|report")
	} else {
		switch args[0] {
		case "run":
			err = loadrun.Command(ctx, args)
		case "queries":
			err = queryfile.Command(ctx, args[1:])
		case "sample":
			err = observe.Sample(ctx, args[1:])
		case "report":
			err = observe.CommandReport(args[1:])
		default:
			err = errors.New("unknown subcommand")
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
