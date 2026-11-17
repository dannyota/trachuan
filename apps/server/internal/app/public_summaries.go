package app

import (
	"context"

	"danny.vn/trachuan/apps/server/internal/publicsummary"
)

func watchPublicSummaries(ctx context.Context, manager *publicsummary.Manager) func() {
	work, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { defer close(done); manager.Watch(work) }()
	return func() { cancel(); <-done }
}
