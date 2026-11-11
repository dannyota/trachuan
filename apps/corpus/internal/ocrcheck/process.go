package ocrcheck

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"syscall"
	"time"
)

func runCommand(ctx context.Context, dir, binary string, args []string, out io.Writer) (RenderStats, error) {
	stats := RenderStats{ExitCode: -1}
	if err := checkScratch(dir); err != nil {
		return stats, err
	}
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = 50 * time.Millisecond
	kill := func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	cmd.Cancel = kill
	cmd.Dir = dir
	cmd.Env = []string{"LC_ALL=C", "HOME=" + dir, "TMPDIR=" + dir}
	var stderr byteCounter
	cmd.Stderr = &stderr
	cmd.Stdout = out
	if err := cmd.Start(); err != nil {
		return stats, renderFailed
	}
	stop := make(chan struct{})
	watched := make(chan error, 1)
	go func() {
		ticker := time.NewTicker(2 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				watched <- nil
				return
			case <-ticker.C:
				if err := checkScratch(dir); err != nil {
					_ = kill()
					watched <- err
					return
				}
			}
		}
	}()
	err := cmd.Wait()
	close(stop)
	// Kill descendants even if a wrapper exited before closing inherited pipes.
	_ = kill()
	stats.ExitCode = cmd.ProcessState.ExitCode()
	stats.StderrBytes = stderr.n
	if limitErr := <-watched; limitErr != nil {
		return stats, limitErr
	}
	if limitErr := checkScratch(dir); limitErr != nil {
		return stats, limitErr
	}
	if err != nil {
		if errors.Is(err, renderLimit) {
			return stats, renderLimit
		}
		return stats, renderFailed
	}
	return stats, nil
}
