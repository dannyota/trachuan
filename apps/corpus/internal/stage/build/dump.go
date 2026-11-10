package build

import (
	"bytes"
	"context"
	"os/exec"
	"time"
)

func pgDump(ctx context.Context, cfg Config, dbName, out string) error {
	env, password, err := toolEnvironment(cfg.BuilderURL)
	if err != nil {
		return err
	}
	timeout, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(timeout, cfg.PgDump, "-Fc", "--no-owner", "--no-privileges", "--no-comments", "--dbname", dbName, "--file", out)
	cmd.Env = env
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err = cmd.Run(); err != nil {
		return toolError("dump corpus database", err, stderr.String(), password, cfg.BuilderURL)
	}
	return nil
}
