package build

import (
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

func TestHiddenError_PostgresIdentifiersOnly(t *testing.T) {
	private := "invented licensed text"
	cause := &pgconn.PgError{
		Code: "23514", TableName: "source_file", ColumnName: "role", ConstraintName: "source_file_role_check",
		Message: private, Detail: private, Hint: private, Where: private, InternalQuery: private,
		SchemaName: private, DataTypeName: private, File: private, Routine: private,
	}
	for name, err := range map[string]error{
		"direct":        cause,
		"wrapped":       fmt.Errorf("%s: %w", private, cause),
		"nested hidden": hide(private, fmt.Errorf("%s: %w", private, cause)),
		"joined":        errors.Join(errors.New(private), cause),
	} {
		t.Run(name, func(t *testing.T) {
			hidden := hide("build database", err)
			require.Equal(t, "build database: pg 23514 table=source_file column=role constraint=source_file_role_check", hidden.Error())
			require.NotContains(t, hidden.Error(), private)
			require.ErrorIs(t, hidden, cause)
			var actual *pgconn.PgError
			require.ErrorAs(t, hidden, &actual)
			require.Same(t, cause, actual)
		})
	}
}

func TestHiddenError_OtherCausesShowTypeOnly(t *testing.T) {
	private := errors.New("invented licensed error")
	for _, tc := range []struct {
		cause error
		want  string
	}{
		{private, "*errors.errorString"},
		{&os.PathError{Op: "invented licensed operation", Path: "invented licensed path", Err: private}, "*fs.PathError"},
		{fmt.Errorf("invented licensed context: %w", private), "*fmt.wrapError"},
		{nil, "<nil>"},
		{(*pgconn.PgError)(nil), "*pgconn.PgError"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			require.Equal(t, tc.want, (&hiddenError{cause: tc.cause}).Error())
		})
	}
}
