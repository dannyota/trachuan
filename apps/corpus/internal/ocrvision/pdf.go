package ocrvision

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

func readPDF(doc Document) ([]byte, string) {
	fd, err := unix.Open(doc.Path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, "failed"
	}
	file := os.NewFile(uintptr(fd), "OCR PDF")
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 50_000_000 {
		return nil, "failed"
	}
	data, err := io.ReadAll(io.LimitReader(file, 50_000_001))
	if err != nil || len(data) > 50_000_000 {
		return nil, "failed"
	}
	if fmt.Sprintf("%x", sha256.Sum256(data)) != doc.Hash {
		return nil, "hash_mismatch"
	}
	return data, ""
}
