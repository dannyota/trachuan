package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"danny.vn/trachuan/apps/server/internal/app"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := app.Run(ctx, os.Args[1:], os.Stderr)
	stop()
	os.Exit(code)
}
