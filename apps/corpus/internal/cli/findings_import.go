package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"danny.vn/trachuan/apps/corpus/internal/intake"
)

func runFindingsImport(args []string, stdout, stderr io.Writer) int {
	usage := func() int {
		_, _ = fmt.Fprintln(stderr, "usage: corpus findings import <cc> <export-directory> [--write]")
		return 2
	}
	write := len(args) == 3 && args[2] == "--write"
	if len(args) != 2 && !write {
		return usage()
	}
	path, err := intake.SourcePath(args[0])
	if err != nil {
		return usage()
	}
	existing, _, err := intake.ParseFile(args[0], path)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	records, err := intake.ReadBatch(args[0], args[1])
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	merged, added, remaining, err := intake.Merge(args[0], existing, records)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	encoded := intake.Encode(merged)
	if _, err = intake.Parse(args[0], encoded); err != nil {
		_, _ = fmt.Fprintf(stderr, "the merged file would not parse, nothing written: %v\n", err)
		return 1
	}
	summary := fmt.Sprintf("%d records in the batch, %d added, %d remaining (run again to continue), %d already present\n", len(records), len(added), remaining, len(records)-len(added)-remaining)
	if !write {
		if len(added) > 0 {
			_, _ = stdout.Write(intake.Encode(added))
		}
		_, _ = fmt.Fprint(stderr, summary)
		return 0
	}
	if len(added) > 0 {
		if err = replaceFile(path, encoded); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return 1
		}
	}
	_, _ = fmt.Fprint(stdout, summary)
	return 0
}

func replaceFile(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".reports-*")
	if err != nil {
		return fmt.Errorf("create temporary file: %w", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err = tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write reports: %w", err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("close reports: %w", err)
	}
	if err = os.Chmod(tmp.Name(), 0o644); err != nil {
		return fmt.Errorf("chmod reports: %w", err)
	}
	if err = os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("replace reports: %w", err)
	}
	return nil
}
