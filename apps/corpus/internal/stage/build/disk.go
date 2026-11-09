package build

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

func checkDisk(path string, minimum uint64) error {
	if path == "" {
		return fmt.Errorf("missing disk path")
	}
	path = filepath.Clean(path)
	for {
		var stats syscall.Statfs_t
		err := syscall.Statfs(path, &stats)
		if err == nil {
			if uint64(stats.Bavail)*uint64(stats.Bsize) < minimum {
				return fmt.Errorf("insufficient free disk")
			}
			return nil
		}
		if !errors.Is(err, syscall.ENOENT) {
			return fmt.Errorf("check free disk: %w", err)
		}
		parent := filepath.Dir(path)
		if parent == path {
			return fmt.Errorf("disk path does not exist")
		}
		path = parent
	}
}

func (c Config) checkDisks(root string, minimum uint64) error {
	check := c.diskCheck
	if check == nil {
		check = checkDisk
	}
	if c.DatabaseDiskPath == "" || root == "" {
		return fmt.Errorf("missing database or artifact disk path")
	}
	info, err := os.Stat(c.DatabaseDiskPath)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("database disk path is not an existing directory")
	}
	if err := check(c.DatabaseDiskPath, minimum); err != nil {
		return fmt.Errorf("database disk: %w", err)
	}
	if err := check(root, minimum); err != nil {
		return fmt.Errorf("artifact disk: %w", err)
	}
	return nil
}

func (c Config) beforeBytes() uint64 {
	if c.MinFreeBeforeBytes == 0 {
		return 30 << 30
	}
	return c.MinFreeBeforeBytes
}

func (c Config) duringBytes() uint64 {
	if c.MinFreeDuringBytes == 0 {
		return 10 << 30
	}
	return c.MinFreeDuringBytes
}
