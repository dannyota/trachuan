package evaluation

import (
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

func sqlState(err error) string {
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		return pg.Code
	}
	return ""
}

type stateError struct {
	err  error
	code string
}

func (e stateError) Error() string { return fmt.Sprintf("%s (SQLSTATE %s)", e.err, e.code) }
func (e stateError) Unwrap() error { return e.err }

func OfflineError(err error) error {
	if code := sqlState(err); code != "" && !strings.Contains(err.Error(), "(SQLSTATE "+code+")") {
		return stateError{err: err, code: code}
	}
	return err
}
