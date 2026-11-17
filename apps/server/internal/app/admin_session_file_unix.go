//go:build linux || darwin

package app

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func duplicateAdminSessionFile(file *os.File) (*os.File, error) {
	fd, err := unix.FcntlInt(file.Fd(), unix.F_DUPFD_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("duplicate session file: %w", err)
	}
	return os.NewFile(uintptr(fd), file.Name()), nil
}
