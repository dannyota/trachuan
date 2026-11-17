package releasecheck

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"danny.vn/trachuan/internal/frameworks/serve"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestFrameworksCheckFailure_OnlyStepAndSafeCause(t *testing.T) {
	marker := "invented protected diagnostic"
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{"engine", serve.ErrUnavailable, "frameworks release-check open engine: Go type serve.failure"},
		{"wrapped Go error", fmt.Errorf("%s: %w", marker, errors.New(marker)), "frameworks release-check open engine: Go type *errors.errorString"},
		{"SQL error", &pgconn.PgError{Code: "42P01", Message: marker, Detail: marker, Hint: marker, TableName: marker}, "frameworks release-check open engine: SQLSTATE 42P01"},
		{"wrapped SQL error", fmt.Errorf("%s: %w", marker, &pgconn.PgError{Code: "42501", Message: marker}), "frameworks release-check open engine: SQLSTATE 42501"},
		{"invalid SQLSTATE", &pgconn.PgError{Code: marker, Message: marker}, "frameworks release-check open engine: Go type *pgconn.PgError"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := frameworksCheckFailure("open engine", tc.err)
			if got.Error() != tc.want {
				t.Fatalf("unexpected safe cause: %s", got)
			}
			if strings.Contains(fmt.Sprintf("%+v %#v", got, got), marker) {
				t.Fatal("retained protected error fields")
			}
			if errors.Unwrap(got) != nil {
				t.Fatal("retained unsafe error chain")
			}
		})
	}
}

func TestCheckFrameworks_ReportsConfigurationStep(t *testing.T) {
	_, err := CheckFrameworks(t.Context(), "invalid", "frameworks_20990101", nil)
	if err == nil || err.Error() != "frameworks release-check configuration: Go type *errors.errorString" {
		t.Fatal("missing safe configuration cause")
	}
}
