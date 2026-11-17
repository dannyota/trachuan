package build

import (
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
)

type hiddenError struct{ cause error }

func (e *hiddenError) Error() string {
	var c *caseFailure
	if errors.As(e.cause, &c) {
		return c.Error()
	}
	var pg *pgconn.PgError
	if errors.As(e.cause, &pg) && pg != nil {
		return fmt.Sprintf("pg %s table=%s column=%s constraint=%s", pg.Code, pg.TableName, pg.ColumnName, pg.ConstraintName)
	}
	return fmt.Sprintf("%T", e.cause)
}

func (e *hiddenError) Unwrap() error { return e.cause }

func hide(message string, err error) error {
	return fmt.Errorf("%s: %w", message, &hiddenError{cause: err})
}
