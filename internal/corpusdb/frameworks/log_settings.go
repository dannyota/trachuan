package frameworks

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

type SettingsConnection interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func CheckLogSettings(ctx context.Context, conn SettingsConnection) error {
	var messages, statements, duration, statement string
	err := conn.QueryRow(ctx, `SELECT current_setting('log_min_messages'),
 current_setting('log_min_error_statement'), current_setting('log_min_duration_statement'),
 current_setting('log_statement')`).Scan(&messages, &statements, &duration, &statement)
	if err != nil {
		return errors.New("frameworks log settings unavailable")
	}
	if messages != "panic" || statements != "panic" || duration != "-1" || statement != "none" {
		return errors.New("frameworks log settings refused")
	}
	return nil
}
