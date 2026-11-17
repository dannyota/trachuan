package evaluate

import (
	"context"

	"danny.vn/trachuan/internal/frameworks/serve"
	"github.com/jackc/pgx/v5"
)

type invariantCopies struct {
	restore Restore
	open    Opener
	mutate  func(context.Context, *pgx.Conn, serve.Caller, string) error
}
