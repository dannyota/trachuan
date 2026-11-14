package seal

import (
	"errors"
	"io"
	"os"
)

func LoadFile(path string) (*Ring, error) {
	invalid := errors.New("invalid or unreadable SERVER_ACCOUNT_KEYS_FILE")
	if path == "" {
		return nil, invalid
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, invalid
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return nil, invalid
	}
	content, err := io.ReadAll(io.LimitReader(file, 1025))
	if err != nil || len(content) > 1024 {
		return nil, invalid
	}
	ring, err := Parse(content)
	if err != nil {
		return nil, invalid
	}
	return ring, nil
}
func (r *Ring) ActiveID() string {
	if r == nil || len(r.keys) == 0 {
		return ""
	}
	return r.keys[0].id
}
