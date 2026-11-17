package build

import (
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
)

type FailureCause struct {
	GoType     string `json:"go_type,omitempty"`
	SQLState   string `json:"sqlstate,omitempty"`
	Table      string `json:"table,omitempty"`
	Column     string `json:"column,omitempty"`
	Constraint string `json:"constraint,omitempty"`
}

func IdentifyFailure(err error) FailureCause {
	if err == nil {
		err = fmt.Errorf("invariant comparison failed")
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg != nil {
		return FailureCause{GoType: fmt.Sprintf("%T", pg), SQLState: pg.Code, Table: pg.TableName, Column: pg.ColumnName, Constraint: pg.ConstraintName}
	}
	for errors.Unwrap(err) != nil {
		err = errors.Unwrap(err)
	}
	return FailureCause{GoType: fmt.Sprintf("%T", err)}
}
