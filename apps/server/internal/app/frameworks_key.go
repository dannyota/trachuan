package app

import (
	"errors"
	"io"
	"os"
)

func loadFrameworksKey(path string) ([]byte, error) {
	invalid := errors.New("invalid or unreadable SERVER_FRAMEWORKS_KEY_FILE")
	file, err := os.Open(path)
	if err != nil {
		return nil, invalid
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return nil, invalid
	}
	key, err := io.ReadAll(file)
	if err != nil || len(key) < 32 {
		return nil, invalid
	}
	return key, nil
}
