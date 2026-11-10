package build

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
)

func watchLock(ctx context.Context, conn *pgx.Conn, cancel context.CancelFunc, diskCheck func() error) func() error {
	stop := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				done <- nil
				return
			case <-ctx.Done():
				done <- ctx.Err()
				return
			case <-ticker.C:
				if diskCheck != nil {
					if err := diskCheck(); err != nil {
						cancel()
						done <- err
						return
					}
				}
				if err := conn.Ping(ctx); err != nil {
					cancel()
					done <- fmt.Errorf("build lock connection lost: %w", err)
					return
				}
			}
		}
	}()
	var once sync.Once
	var result error
	return func() error { once.Do(func() { close(stop); result = <-done }); return result }
}

func watchError(result, watch error) error {
	if watch != nil && !errors.Is(watch, context.Canceled) {
		return watch
	}
	return result
}
