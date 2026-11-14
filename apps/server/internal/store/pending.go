package store

import (
	"context"
	"fmt"
)

type PendingParams struct {
	Next           string
	ReplacedHashes []string
}
type PendingRequired struct{ CSRF string }

func (*PendingRequired) Error() string { return "factor required" }

func (q *Queries) primaryPending(ctx context.Context, p CreateSessionParams, pending PendingParams, kind string) (bool, error) {
	enrolled, err := q.HasTOTP(ctx, p.AccountID)
	if err != nil {
		return false, fmt.Errorf("read primary factor: %w", err)
	}
	if _, err = q.db.Exec(ctx, `UPDATE pending_auth SET consumed_at=statement_timestamp() WHERE token_hash=ANY($1::text[]) AND consumed_at IS NULL`, pending.ReplacedHashes); err != nil {
		return false, fmt.Errorf("consume browser pending authentication: %w", err)
	}
	if !enrolled {
		return false, nil
	}
	if _, err = q.db.Exec(ctx, `INSERT INTO pending_auth(token_hash,csrf_secret,account_id,auth_epoch,primary_kind,primary_at,next,expires_at)
 VALUES($1,$2,$3,$4,$5,statement_timestamp(),$6,statement_timestamp()+interval '5 minutes')`, p.TokenHash, p.CsrfSecret, p.AccountID, p.AuthEpoch, kind, pending.Next); err != nil {
		return false, fmt.Errorf("create pending authentication: %w", err)
	}
	return true, nil
}
