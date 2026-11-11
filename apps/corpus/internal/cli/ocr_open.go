package cli

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

type rejectedOCR struct{}

func (rejectedOCR) Error() string { return "invalid OCR import file" }

func openImportAt(dir *os.File, name string) (*os.File, error) {
	fd, err := unix.Openat(int(dir.Fd()), name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if errors.Is(err, unix.ELOOP) {
		return nil, rejectedOCR{}
	}
	if err != nil {
		return nil, fmt.Errorf("open OCR import: %w", err)
	}
	f := os.NewFile(uintptr(fd), name)
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("stat OCR import: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > importTextLimit {
		_ = f.Close()
		return nil, rejectedOCR{}
	}
	return f, nil
}
