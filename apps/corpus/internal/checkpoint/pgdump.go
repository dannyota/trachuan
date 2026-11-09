package checkpoint

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
)

// PgDump invokes PostgreSQL's custom-format dump tool.
type PgDump struct{ Path string }

// Dump writes a custom-format dump without logging the database URL.
func (p PgDump) Dump(ctx context.Context, databaseURL, path string) error {
	command := exec.CommandContext(ctx, p.Path, "-Fc", "--file", path, databaseURL)
	if err := command.Run(); err != nil {
		return fmt.Errorf("run pg_dump: %w", err)
	}
	return nil
}

// CheckVersion requires pg_dump major version 18 or newer.
func CheckVersion(ctx context.Context, path string) error {
	return checkVersion(ctx, path, func(ctx context.Context, path string) (string, error) {
		out, err := exec.CommandContext(ctx, path, "--version").Output()
		return string(out), err
	})
}
func checkVersion(ctx context.Context, path string, version func(context.Context, string) (string, error)) error {
	output, err := version(ctx, path)
	if err != nil {
		return fmt.Errorf("get pg_dump version: %w", err)
	}
	match := regexp.MustCompile(`^pg_dump \(PostgreSQL\) ([0-9]+)(\.|\s|$)`).FindStringSubmatch(output)
	if len(match) != 3 {
		return fmt.Errorf("unrecognized pg_dump version %q", output)
	}
	major, err := strconv.Atoi(match[1])
	if err != nil {
		return fmt.Errorf("parse pg_dump version: %w", err)
	}
	if major < 18 {
		return fmt.Errorf("pg_dump major version %d is below 18", major)
	}
	return nil
}
