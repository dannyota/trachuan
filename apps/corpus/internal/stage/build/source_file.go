package build

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
)

func checkSourceFile(path string, size int64, expected string) error {
	decoded, err := hex.DecodeString(expected)
	if err != nil || len(decoded) != 32 || fmt.Sprintf("%x", decoded) != expected {
		return fmt.Errorf("invalid source file hash")
	}
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open source file: %w", err)
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stat source file: %w", err)
	}
	if info.Size() != size || size < 0 {
		return fmt.Errorf("source file size mismatch")
	}
	hash := sha256.New()
	count, err := io.Copy(hash, io.LimitReader(f, size+1))
	if err != nil {
		return fmt.Errorf("hash source file: %w", err)
	}
	if count != size || fmt.Sprintf("%x", hash.Sum(nil)) != expected {
		return fmt.Errorf("source file hash mismatch")
	}
	return nil
}
