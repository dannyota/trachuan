package store

import (
	"context"
	"fmt"
	"time"

	"danny.vn/trachuan/apps/server/internal/seal"
	"github.com/jackc/pgx/v5"
)

type EnqueueMailParams struct {
	Kind, Class            string
	AccountID, EmailLinkID *int64
	Payload                []byte
}

func EnqueueMail(ctx context.Context, tx pgx.Tx, ring *seal.Ring, p EnqueueMailParams) (int64, error) {
	if (p.AccountID == nil) == (p.EmailLinkID == nil) || len(p.Payload) == 0 {
		return 0, fmt.Errorf("invalid mail owner or payload")
	}
	q := New(tx)
	id, err := q.AllocateMailID(ctx)
	if err != nil {
		return 0, fmt.Errorf("allocate mail row: %w", err)
	}
	row := MailOutbox{ID: id, AccountID: p.AccountID, EmailLinkID: p.EmailLinkID}
	sealed, keyID, err := ring.Seal(MailBinding(row), p.Payload)
	if err != nil {
		return 0, fmt.Errorf("seal mail: %w", err)
	}
	id, err = q.InsertSealedMail(ctx, InsertSealedMailParams{ID: id, Kind: p.Kind, Class: p.Class, AccountID: p.AccountID, EmailLinkID: p.EmailLinkID, SealedPayload: sealed, KeyID: keyID})
	if err != nil {
		return 0, fmt.Errorf("insert sealed mail: %w", err)
	}
	return id, nil
}
func MailBinding(row MailOutbox) seal.Binding {
	if row.AccountID != nil {
		return seal.Binding{Purpose: "mail", OwnerKind: "account", OwnerID: *row.AccountID, RowID: row.ID}
	}
	if row.EmailLinkID != nil {
		return seal.Binding{Purpose: "mail", OwnerKind: "email_link", OwnerID: *row.EmailLinkID, RowID: row.ID}
	}
	return seal.Binding{}
}
func (q *Queries) OpenClaimedMail(ctx context.Context, ring *seal.Ring, claim MailOutbox) ([]byte, error) {
	db, ok := q.db.(TransactionBeginner)
	if !ok {
		return nil, fmt.Errorf("mail store cannot begin transactions")
	}
	tx, err := db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin mail open: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	current := New(tx)
	row, err := current.LockClaimedMail(ctx, LockClaimedMailParams{ID: claim.ID, Attempts: claim.Attempts})
	if err != nil {
		return nil, fmt.Errorf("lock claimed mail: %w", err)
	}
	plain, previous, err := ring.Open(MailBinding(row), row.KeyID, row.SealedPayload)
	if err != nil {
		return nil, fmt.Errorf("open mail: %w", err)
	}
	if previous {
		payload, keyID, sealErr := ring.Seal(MailBinding(row), plain)
		if sealErr != nil {
			return nil, fmt.Errorf("reseal mail: %w", sealErr)
		}
		if err = current.ResealMail(ctx, ResealMailParams{ID: row.ID, SealedPayload: payload, KeyID: keyID}); err != nil {
			return nil, fmt.Errorf("write resealed mail: %w", err)
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit mail open: %w", err)
	}
	return plain, nil
}

type FinishMailParams struct {
	ID         int64
	Attempts   int32
	Now        time.Time
	Sent, Drop bool
}

func (q *Queries) FinishMail(ctx context.Context, p FinishMailParams) error {
	next := p.Now.Add(time.Minute * time.Duration(1<<min(max(p.Attempts-1, 0), 5)))
	_, err := q.db.Exec(ctx, `UPDATE mail_outbox SET
 sent_at=CASE WHEN $4 THEN $3::timestamptz ELSE sent_at END,
 sealed_payload=CASE WHEN $4 OR $5 OR attempts>=6 OR expires_at<=$3::timestamptz THEN NULL ELSE sealed_payload END,
 next_attempt_at=$6,claimed_until=NULL
 WHERE id=$1 AND attempts=$2 AND sent_at IS NULL`, p.ID, p.Attempts, p.Now, p.Sent, p.Drop, next)
	if err != nil {
		return fmt.Errorf("finish mail: %w", err)
	}
	return nil
}
