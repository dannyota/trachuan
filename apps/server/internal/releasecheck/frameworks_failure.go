package releasecheck

import (
	"errors"
	"fmt"
	"regexp"

	"github.com/jackc/pgx/v5/pgconn"
)

func frameworksCheckFailure(step string, err error) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg != nil && regexp.MustCompile(`^[0-9A-Z]{5}$`).MatchString(pg.Code) {
		return fmt.Errorf("frameworks release-check %s: SQLSTATE %s", step, pg.Code)
	}
	for errors.Unwrap(err) != nil {
		err = errors.Unwrap(err)
	}
	return fmt.Errorf("frameworks release-check %s: Go type %T", step, err)
}
