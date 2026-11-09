package artifact

import (
	"context"
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"
)

func lock(ctx context.Context, path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("open lock: %w", err)
	}
	for {
		if err = ctx.Err(); err != nil {
			_ = f.Close()
			return nil, fmt.Errorf("wait for lock: %w", err)
		}
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return f, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
			_ = f.Close()
			return nil, fmt.Errorf("lock artifact: %w", err)
		}
		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			_ = f.Close()
			return nil, fmt.Errorf("wait for lock: %w", ctx.Err())
		case <-timer.C:
		}
	}
}
