package accountflow

import (
	"context"
	"errors"
	"fmt"
	"time"

	"danny.vn/trachuan/apps/server/internal/store"
	"github.com/jackc/pgx/v5"
)

const recentWindow = 10 * time.Minute

type Link struct {
	ID                   int64
	Purpose, Email       string
	AccountID            *int64
	AuthEpoch            *int32
	ConsumedAt           *time.Time
	ExpiresAt, CheckedAt time.Time
}

func readLink(ctx context.Context, db store.DBTX, digest string, lock bool) (Link, error) {
	query := `SELECT id,purpose,email,account_id,auth_epoch,consumed_at,expires_at,statement_timestamp() FROM email_link WHERE token_hash=$1`
	if lock {
		query += " FOR UPDATE"
	}
	var l Link
	err := db.QueryRow(ctx, query, digest).Scan(&l.ID, &l.Purpose, &l.Email, &l.AccountID, &l.AuthEpoch, &l.ConsumedAt, &l.ExpiresAt, &l.CheckedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return l, ErrLinkInvalid
	}
	if err != nil {
		return l, fmt.Errorf("read email link: %w", err)
	}
	if l.ConsumedAt != nil || !l.ExpiresAt.After(l.CheckedAt) {
		return l, ErrLinkInvalid
	}
	return l, nil
}
func (s *Service) LookupLink(ctx context.Context, digest string) (Link, error) {
	var link Link
	err := s.transaction(ctx, func(tx pgx.Tx) error {
		var err error
		link, err = readLink(ctx, tx, digest, false)
		return err
	})
	return link, err
}
func claim(ctx context.Context, tx pgx.Tx, digest, purpose string, account *store.Account) (Link, error) {
	l, err := readLink(ctx, tx, digest, true)
	if err != nil {
		return l, err
	}
	if l.Purpose != purpose {
		return l, ErrLinkInvalid
	}
	if account != nil && (l.AccountID == nil || l.AuthEpoch == nil || *l.AccountID != account.ID || *l.AuthEpoch != account.AuthEpoch || l.Email != account.Email) {
		return l, ErrLinkInvalid
	}
	return l, nil
}
func consume(ctx context.Context, tx pgx.Tx, id int64) error {
	if _, err := tx.Exec(ctx, `UPDATE email_link SET consumed_at=statement_timestamp() WHERE id=$1`, id); err != nil {
		return fmt.Errorf("consume email link: %w", err)
	}
	return nil
}
