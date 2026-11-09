package build

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

func filterRestoreList(list string) (string, error) {
	var result strings.Builder
	for _, line := range strings.SplitAfter(list, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, ";") {
			result.WriteString(line)
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) < 4 || !strings.HasSuffix(fields[0], ";") {
			return "", fmt.Errorf("invalid restore list entry")
		}
		switch fields[3] {
		case "EXTENSION":
			if len(fields) < 6 || (fields[5] != "vector" && fields[5] != "ltree") {
				return "", fmt.Errorf("unexpected extension in restore list")
			}
			continue
		case "FUNCTION", "PROCEDURE":
			return "", fmt.Errorf("function in restore list")
		case "EVENT":
			if len(fields) > 4 && fields[4] == "TRIGGER" {
				return "", fmt.Errorf("event trigger in restore list")
			}
		}
		result.WriteString(line)
	}
	return result.String(), nil
}

func pgRestore(ctx context.Context, cfg Config, dbName, dumpPath string) error {
	env, password, err := toolEnvironment(cfg.BuilderURL)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()
	listCmd := exec.CommandContext(ctx, cfg.PgRestore, "-l", dumpPath)
	listCmd.Env = env
	var list, stderr bytes.Buffer
	listCmd.Stdout, listCmd.Stderr = &list, &stderr
	if err = listCmd.Run(); err != nil {
		return toolError("list corpus dump", err, stderr.String(), password, cfg.BuilderURL)
	}
	filtered, err := filterRestoreList(list.String())
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, cfg.PgRestore, "--no-owner", "--no-privileges", "--exit-on-error", "--use-list", "/dev/stdin", "--dbname", dbName, dumpPath)
	cmd.Env, cmd.Stdin = env, strings.NewReader(filtered)
	stderr.Reset()
	cmd.Stderr = &stderr
	if err = cmd.Run(); err != nil {
		return toolError("restore corpus database", err, stderr.String(), password, cfg.BuilderURL)
	}
	return nil
}
