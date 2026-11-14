package releasecheck

import (
	"fmt"
	"regexp"
	"strconv"
	"testing"
)

func TestPGClientMajorVersion(t *testing.T) {
	for _, tc := range []struct {
		program string
		output  string
		want    int
		wantErr bool
	}{
		{"pg_dump", "pg_dump (PostgreSQL) 17.6\n", 17, false},
		{"pg_restore", "pg_restore (PostgreSQL) 18.1 (Homebrew)\n", 18, false},
		{"pg_dump", "pg_dump (PostgreSQL) 19\n", 19, false},
		{"pg_dump", "pg_restore (PostgreSQL) 18.1\n", 0, true},
		{"pg_dump", "pg_dump (PostgreSQL) invalid\n", 0, true},
	} {
		t.Run(tc.output, func(t *testing.T) {
			got, err := pgClientMajorVersion(tc.program, tc.output)
			if (err != nil) != tc.wantErr || got != tc.want {
				t.Fatalf("major = %d, error = %v; want %d, error = %t", got, err, tc.want, tc.wantErr)
			}
		})
	}
}

func pgClientMajorVersion(program, output string) (int, error) {
	match := regexp.MustCompile(`^` + regexp.QuoteMeta(program) + ` \(PostgreSQL\) ([0-9]+)(\.|\s|$)`).FindStringSubmatch(output)
	if len(match) != 3 {
		return 0, fmt.Errorf("unrecognized %s version %q", program, output)
	}
	major, err := strconv.Atoi(match[1])
	if err != nil {
		return 0, fmt.Errorf("parse %s version: %w", program, err)
	}
	return major, nil
}
