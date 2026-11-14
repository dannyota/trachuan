package accountflow

import (
	"context"
	"errors"
	"fmt"

	"danny.vn/trachuan/apps/server/internal/store"
	"github.com/jackc/pgx/v5"
)

func (s *Service) CompleteRegistration(ctx context.Context, p Completion) (store.CreateSessionRow, error) {
	var session store.CreateSessionRow
	err := s.transaction(ctx, func(tx pgx.Tx) error {
		link, err := claim(ctx, tx, p.TokenHash, "registration", nil)
		if err != nil {
			return err
		}
		if !s.admits(link.Email) {
			return ErrLinkInvalid
		}
		var id int64
		err = tx.QueryRow(ctx, `INSERT INTO account(email,last_login_at) VALUES ($1,statement_timestamp()) ON CONFLICT(email) DO NOTHING RETURNING id`, link.Email).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrLinkInvalid
		}
		if err != nil {
			return fmt.Errorf("create registered account: %w", err)
		}
		if _, err = store.LockAccount(ctx, tx, id); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO password_credential(account_id,hash) VALUES ($1,$2)`, id, p.Hash); err != nil {
			return fmt.Errorf("create password credential: %w", err)
		}
		if err = consume(ctx, tx, link.ID); err != nil {
			return err
		}
		p.Session.AccountID, p.Session.AuthEpoch = id, 0
		session, err = store.New(tx).CreateSession(ctx, p.Session)
		if err != nil {
			return fmt.Errorf("create registration session: %w", err)
		}
		return nil
	})
	return session, err
}
