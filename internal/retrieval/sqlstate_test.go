package retrieval

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestDatabaseFailure_RetainsSQLStateSafely(t *testing.T) {
	err := databaseFailure("read dense candidates failed", &pgconn.PgError{Code: "57014", Message: "private"})
	var pg *pgconn.PgError
	if !errors.As(err, &pg) || pg.Code != "57014" || pg.Message != "" || err.Error() != "read dense candidates failed" || !errors.Is(err, ErrUnavailable) {
		t.Fatalf("failure = %v, pg = %+v", err, pg)
	}
}
