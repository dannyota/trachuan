package serve

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

type Metadata struct {
	Release           string `json:"release"`
	AccessSetVersion  string `json:"access_set_version"`
	AccessUnavailable bool   `json:"access_unavailable,omitempty"`
}

const (
	ErrInvalidHandle failure = "invalid_handle"
	ErrInvalidNext   failure = "invalid_next"
)

func (e *Engine) metadata(p projection, caller Caller) Metadata {
	return Metadata{e.release, p.version, caller.AccessUnavailable}
}

func (e *Engine) beginOperation(ctx context.Context) (context.Context, pgx.Tx, context.CancelFunc, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	if e.db == nil {
		cancel()
		return ctx, nil, cancel, ErrUnavailable
	}
	tx, err := e.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		cancel()
		return ctx, nil, cancel, ErrUnavailable
	}
	if _, err = tx.Exec(ctx, transactionSettingsSQL); err != nil {
		_ = tx.Rollback(context.Background())
		cancel()
		return ctx, nil, cancel, ErrUnavailable
	}
	return ctx, tx, cancel, nil
}

func (e *Engine) releaseMatches(release string) bool { return release == "" || release == e.release }

func (e *Engine) versionFor(h handle) (version, bool) {
	for _, v := range e.versions {
		if v.info.Code == h.code && v.info.Version == h.version {
			return v, true
		}
	}
	return version{}, false
}
