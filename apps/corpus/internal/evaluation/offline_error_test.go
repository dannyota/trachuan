package evaluation

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

type databaseError struct{ code string }

func (e databaseError) Error() string { return "read embedding dimensions failed" }
func (e databaseError) As(target any) bool {
	if pg, ok := target.(**pgconn.PgError); ok {
		*pg = &pgconn.PgError{Code: e.code}
		return true
	}
	return false
}

func TestOfflineError_SQLStateOnceAcrossBoundaries(t *testing.T) {
	for _, prefix := range []string{"open draft corpus", "open retrieval"} {
		cause := databaseError{code: "57014"}
		err := OfflineError(fmt.Errorf("%s: %w", prefix, OfflineError(cause)))
		want := prefix + ": read embedding dimensions failed (SQLSTATE 57014)"
		if err.Error() != want {
			t.Fatalf("got %q, want %q", err, want)
		}
		var pg *pgconn.PgError
		if !errors.As(err, &pg) || pg.Code != "57014" {
			t.Fatalf("missing SQLSTATE: %v", err)
		}
	}
}

func TestOfflineError_SQLStateAppearsOnce(t *testing.T) {
	pg := &pgconn.PgError{Code: "57014", Severity: "ERROR", Message: "cancelled"}
	for _, tc := range []struct {
		name  string
		cause error
		want  string
	}{
		{"native", pg, "ERROR: cancelled (SQLSTATE 57014)"},
		{"wrapped", fmt.Errorf("read release: %w", pg), "read release: ERROR: cancelled (SQLSTATE 57014)"},
		{"sanitized", databaseError{code: "57014"}, "read embedding dimensions failed (SQLSTATE 57014)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := OfflineError(tc.cause)
			if got.Error() != tc.want || strings.Count(got.Error(), "SQLSTATE 57014") != 1 {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
			var state *pgconn.PgError
			if !errors.As(got, &state) || state.Code != "57014" {
				t.Fatalf("SQLSTATE lost: %v", got)
			}
		})
	}
}
