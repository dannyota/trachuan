package build

import (
	"fmt"
	"regexp"
	"strings"
)

func sanitizeToolStderr(stderr, password, connection string) string {
	for _, secret := range []string{connection, password} {
		if secret != "" {
			stderr = strings.ReplaceAll(stderr, secret, "[redacted]")
		}
	}
	stderr = regexp.MustCompile(`(?i)postgres(?:ql)?://[^\s]+`).ReplaceAllString(stderr, "[redacted]")
	stderr = regexp.MustCompile(`(?i)(host|hostaddr|port|user|dbname|password)\s*=\s*('[^']*'|\S+)`).ReplaceAllString(stderr, "[connection detail redacted]")
	stderr = strings.TrimSpace(stderr)
	if len(stderr) > 2048 {
		stderr = stderr[:2048]
	}
	return stderr
}

func toolError(name string, err error, stderr, password, connection string) error {
	detail := sanitizeToolStderr(stderr, password, connection)
	if detail == "" {
		return fmt.Errorf("%s: %w", name, err)
	}
	return fmt.Errorf("%s: %w: %s", name, err, detail)
}
