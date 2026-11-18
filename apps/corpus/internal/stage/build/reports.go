package build

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"danny.vn/trachuan/apps/corpus/internal/intake"
	"danny.vn/trachuan/apps/corpus/internal/stage"
)

const reportsFileName = "reports.yaml"

type ReportsInput struct {
	file intake.File
	data []byte
	hash string
}

func ReadReports(country, path string) (ReportsInput, error) {
	file, data, err := intake.ParseFile(country, path)
	if err != nil {
		return ReportsInput{}, err
	}
	if file.Len() == 0 {
		return ReportsInput{}, nil
	}
	return ReportsInput{file: file, data: data, hash: hashBytes(data)}, nil
}

func (r ReportsInput) Empty() bool { return r.file.Len() == 0 }

func withReports(input, hash string) string {
	if hash == "" {
		return input
	}
	return stage.RecipeHash("build-reports", intake.RulesVersion, input, hash)
}

func writeReportsArtifact(dir string, input ReportsInput) error {
	if input.Empty() {
		return nil
	}
	if err := os.WriteFile(filepath.Join(dir, reportsFileName), input.data, 0600); err != nil {
		return fmt.Errorf("write reports: %w", err)
	}
	return nil
}

func reportsArtifactHash(dir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(dir, reportsFileName))
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read reports: %w", err)
	}
	return hashBytes(data), nil
}
