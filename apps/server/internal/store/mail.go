package store

import (
	"context"
	"fmt"
	"time"

	"danny.vn/trachuan/apps/server/internal/seal"
	"github.com/jackc/pgx/v5"
)

type MailLink struct {
	Purpose, TokenHash, Email string
	AccountID                 *int64
	AuthEpoch                 *int32
	Lifetime                  time.Duration
}
type EnqueueMailParams struct {
	Kind, Class            string
	AccountID, EmailLinkID *int64
	Payload                []byte
	Link                   *MailLink
	Refused                bool
}

func EnqueueMail(ctx context.Context, tx pgx.Tx, ring *seal.Ring, p EnqueueMailParams) (int64, error) {
	if p.Link == nil && (p.AccountID == nil) == (p.EmailLinkID == nil) || len(p.Payload) == 0 {
		return 0, fmt.Errorf("invalid mail owner or payload")
	}
	var id, linkID int64
	err := tx.QueryRow(ctx, `SELECT nextval(pg_get_serial_sequence('mail_outbox','id')),nextval(pg_get_serial_sequence('email_link','id'))`).Scan(&id, &linkID)
	if err != nil {
		return 0, fmt.Errorf("allocate mail rows: %w", err)
	}
	var purpose, digest, email string
	var owner *int64
	var epoch *int32
	var seconds int64
	if p.Link != nil {
		purpose, digest, email = p.Link.Purpose, p.Link.TokenHash, p.Link.Email
		owner, epoch = p.Link.AccountID, p.Link.AuthEpoch
		seconds = int64(p.Link.Lifetime / time.Second)
		if p.AccountID == nil {
			p.EmailLinkID = &linkID
		}
	}
	row := MailOutbox{ID: id, AccountID: p.AccountID, EmailLinkID: p.EmailLinkID}
	sealed, keyID, err := ring.Seal(MailBinding(row), p.Payload)
	if err != nil {
		return 0, fmt.Errorf("seal mail: %w", err)
	}
	_, err = tx.Exec(ctx, `WITH superseded AS (
  UPDATE email_link SET consumed_at=statement_timestamp() WHERE $8::boolean AND $9::text<>'' AND purpose=$9
   AND consumed_at IS NULL AND (($12::bigint IS NOT NULL AND account_id=$12) OR ($12 IS NULL AND account_id IS NULL AND email=$11))
 ), link AS (
  INSERT INTO email_link(id,purpose,token_hash,email,account_id,auth_epoch,expires_at) OVERRIDING SYSTEM VALUE
  SELECT $15,$9,$10,$11,$12,$13,statement_timestamp()+$14::bigint*interval '1 second' WHERE $8 AND $9<>'' RETURNING id
 )
 INSERT INTO mail_outbox(id,kind,class,account_id,email_link_id,sealed_payload,key_id,expires_at) OVERRIDING SYSTEM VALUE
 SELECT $1,$2,$3,$4,$5,$6,$7,statement_timestamp()+interval '24 hours' WHERE $8
 AND ($9='' OR EXISTS(SELECT 1 FROM link))`, id, p.Kind, p.Class, p.AccountID, p.EmailLinkID, sealed, keyID, !p.Refused, purpose, digest, email, owner, epoch, seconds, linkID)
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
	Uncertain  bool
}

func (q *Queries) FinishMail(ctx context.Context, p FinishMailParams) error {
	next := p.Now.Add(time.Minute * time.Duration(1<<min(max(p.Attempts-1, 0), 5)))
	_, err := q.db.Exec(ctx, `WITH finished AS (UPDATE mail_outbox SET
 sent_at=CASE WHEN $4 THEN $3::timestamptz ELSE sent_at END,
 sealed_payload=CASE WHEN $4 OR $5 OR attempts>=6 OR expires_at<=$3::timestamptz THEN NULL ELSE sealed_payload END,
 next_attempt_at=$6,claimed_until=NULL
 WHERE id=$1 AND attempts=$2 AND sent_at IS NULL RETURNING id
 ), delivered AS (
 UPDATE mail_delivery SET sent_at=$3 WHERE outbox_id IN (SELECT id FROM finished) AND attempt=$2 AND $4
 ) DELETE FROM mail_delivery WHERE outbox_id IN (SELECT id FROM finished) AND attempt=$2 AND NOT $4 AND NOT $7`, p.ID, p.Attempts, p.Now, p.Sent, p.Drop, next, p.Uncertain)
	if err != nil {
		return fmt.Errorf("finish mail: %w", err)
	}
	return nil
}
