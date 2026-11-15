package testdb

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"

	"danny.vn/trachuan/apps/corpus/migrations"
)

func migrationTemplateName() (string, error) {
	files, err := fs.Glob(migrations.FS, "*.sql")
	if err != nil {
		return "", fmt.Errorf("list migrations: %w", err)
	}
	hash := sha256.New()
	for _, name := range files {
		data, err := migrations.FS.ReadFile(name)
		if err != nil {
			return "", fmt.Errorf("read migration: %w", err)
		}
		_, _ = hash.Write(data)
	}
	return "corpus_tpl_" + hex.EncodeToString(hash.Sum(nil))[:12], nil
}
