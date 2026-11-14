package accountflow

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"danny.vn/trachuan/apps/server/internal/authmail"
	"danny.vn/trachuan/apps/server/internal/store"
	"github.com/jackc/pgx/v5"
)

func (s *Service) RequestMail(ctx context.Context, purpose, email string) error {
	return s.transaction(ctx, func(tx pgx.Tx) error {
		if err := mailLock(ctx, tx); err != nil {
			return err
		}
		var lockedOwner int64
		if purpose == "reset" {
			var ownerID int64
			err := tx.QueryRow(ctx, `SELECT id FROM account WHERE email=$1`, email).Scan(&ownerID)
			if err == nil {
				if _, err = store.LockAccount(ctx, tx, ownerID); err != nil {
					return err
				}
				lockedOwner = ownerID
			} else if !errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("find reset owner: %w", err)
			}
		}
		var id *int64
		var epoch *int32
		var recipient string
		var hasPassword, available bool
		err := tx.QueryRow(ctx, `SELECT a.id,a.auth_epoch,COALESCE(a.email,$1),EXISTS(SELECT 1 FROM password_credential p WHERE p.account_id=a.id),
   a.id IS NULL OR (a.blocked_at IS NULL AND a.delete_requested_at IS NULL)
   FROM (SELECT 1) seed LEFT JOIN account a ON a.email=$1`, email).Scan(&id, &epoch, &recipient, &hasPassword, &available)
		if err != nil {
			return fmt.Errorf("look up mail recipient: %w", err)
		}
		kind, class := purpose, "recovery"
		admitted := available && email != ""
		switch purpose {
		case "registration":
			class = "signup"
			if id != nil {
				kind = "account_exists"
			} else {
				admitted = admitted && s.admits(email)
			}
		case "reset":
			admitted = admitted && id != nil && *id == lockedOwner && hasPassword
		default:
			return ErrInvalidRequest
		}
		admitted, err = mailAdmission(ctx, tx, recipient, admitted, kind == "registration")
		if err != nil {
			return err
		}
		return s.enqueueLink(ctx, tx, kind, class, recipient, id, epoch, admitted)
	})
}
func mailLock(ctx context.Context, tx pgx.Tx) error {
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(739215,1)`); err != nil {
		return fmt.Errorf("lock mail admission: %w", err)
	}
	return nil
}
func mailAdmission(ctx context.Context, tx pgx.Tx, email string, admit, registration bool) (bool, error) {
	var allowed bool
	err := tx.QueryRow(ctx, `SELECT $2::boolean
  AND (SELECT count(*) FROM mail_outbox WHERE sent_at IS NULL AND class<>'notice')<5000
  AND (NOT $3::boolean OR (SELECT count(*) FROM email_link WHERE purpose='registration' AND consumed_at IS NULL AND expires_at>statement_timestamp())<10000)
  AND (SELECT count(*) FROM mail_outbox m LEFT JOIN account a ON a.id=m.account_id LEFT JOIN email_link l ON l.id=m.email_link_id
       WHERE COALESCE(a.email,l.email)=$1
       AND m.class<>'notice'
       AND m.created_at>statement_timestamp()-interval '1 hour')<3
  AND (SELECT count(*) FROM mail_outbox m LEFT JOIN account a ON a.id=m.account_id LEFT JOIN email_link l ON l.id=m.email_link_id
       WHERE COALESCE(a.email,l.email)=$1
       AND m.class<>'notice'
       AND m.created_at>statement_timestamp()-interval '1 day')<10`, email, admit, registration).Scan(&allowed)
	if err != nil {
		return false, fmt.Errorf("check mail admission: %w", err)
	}
	return allowed, nil
}
func (s *Service) enqueueLink(ctx context.Context, tx pgx.Tx, kind, class, email string, id *int64, epoch *int32, admitted bool) error {
	token := make([]byte, 32)
	if _, err := rand.Read(token); err != nil {
		return fmt.Errorf("create link token: %w", err)
	}
	raw := base64.RawURLEncoding.EncodeToString(token)
	sum := sha256.Sum256([]byte(raw))
	payload, err := json.Marshal(authmail.Payload{To: email, Token: raw})
	if err != nil {
		return fmt.Errorf("encode link mail: %w", err)
	}
	p := store.EnqueueMailParams{Kind: kind, Class: class, AccountID: id, Payload: payload, Refused: !admitted}
	if kind != "account_exists" {
		lifetime := time.Hour
		if kind == "registration" {
			lifetime = 24 * time.Hour
		}
		p.Link = &store.MailLink{Purpose: kind, TokenHash: hex.EncodeToString(sum[:]), Email: email, AccountID: id, AuthEpoch: epoch, Lifetime: lifetime}
	}
	_, err = store.EnqueueMail(ctx, tx, s.Ring, p)
	return err
}
func (s *Service) RequestAddPassword(ctx context.Context, p Acting) error {
	return s.transaction(ctx, func(tx pgx.Tx) error {
		if err := mailLock(ctx, tx); err != nil {
			return err
		}
		a, err := liveAccount(ctx, tx, p)
		if err != nil {
			return err
		}
		if _, err = liveSession(ctx, tx, p, true); err != nil {
			return err
		}
		_, err = store.New(tx).GetPasswordCredential(ctx, a.ID)
		if err == nil {
			return ErrInvalidRequest
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("read password credential: %w", err)
		}
		admitted, err := mailAdmission(ctx, tx, a.Email, true, false)
		if err != nil {
			return err
		}
		return s.enqueueLink(ctx, tx, "add_password", "recovery", a.Email, &a.ID, &a.AuthEpoch, admitted)
	})
}
