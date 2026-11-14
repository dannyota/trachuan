package store

import (
	"context"
	"fmt"

	"danny.vn/trachuan/apps/server/internal/seal"
	"github.com/jackc/pgx/v5"
)

type sealedTable struct{ table, column, purpose string }

func sealedTables() []sealedTable {
	return []sealedTable{{"totp_credential", "sealed_secret", "totp"}, {"totp_enrollment", "sealed_secret", "totp_enrollment"}, {"mail_outbox", "sealed_payload", "mail"}}
}
func (q *Queries) CheckAccountKeys(ctx context.Context, ring *seal.Ring) (map[string]int64, error) {
	counts := make(map[string]int64)
	for _, table := range sealedTables() {
		var count int64
		query := fmt.Sprintf("SELECT count(*) FROM %s WHERE %s IS NOT NULL AND key_id<>$1", table.table, table.column)
		if err := q.db.QueryRow(ctx, query, ring.ActiveID()).Scan(&count); err != nil {
			return nil, fmt.Errorf("check %s keys: %w", table.table, err)
		}
		counts[table.table] = count
	}
	return counts, nil
}
func (q *Queries) ResealAccountKeys(ctx context.Context, ring *seal.Ring) error {
	db, ok := q.db.(TransactionBeginner)
	if !ok {
		return fmt.Errorf("key store cannot begin transactions")
	}
	for _, table := range sealedTables() {
		var cursor int64
		for {
			id, err := resealRow(ctx, db, ring, table, cursor)
			if err != nil {
				return err
			}
			if id == 0 {
				break
			}
			cursor = id
		}
	}
	return nil
}
func resealRow(ctx context.Context, db TransactionBeginner, ring *seal.Ring, table sealedTable, cursor int64) (int64, error) {
	tx, err := db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return 0, fmt.Errorf("begin key reseal: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var id int64
	var owner, link *int64
	var data []byte
	var keyID string
	linkColumn := "NULL::bigint"
	if table.table == "mail_outbox" {
		linkColumn = "email_link_id"
	}
	query := fmt.Sprintf("SELECT id,account_id,%s,%s,key_id FROM %s WHERE id>$1 AND %s IS NOT NULL ORDER BY id LIMIT 1 FOR UPDATE", linkColumn, table.column, table.table, table.column)
	err = tx.QueryRow(ctx, query, cursor).Scan(&id, &owner, &link, &data, &keyID)
	if err == pgx.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read %s sealed row: %w", table.table, err)
	}
	binding := seal.Binding{Purpose: table.purpose, OwnerKind: "account", RowID: id}
	if owner != nil {
		binding.OwnerID = *owner
	} else if link != nil {
		binding.OwnerKind = "email_link"
		binding.OwnerID = *link
	}
	plain, previous, err := ring.Open(binding, keyID, data)
	if err != nil {
		return 0, fmt.Errorf("open %s sealed row: %w", table.table, err)
	}
	if previous {
		sealed, active, sealErr := ring.Seal(binding, plain)
		if sealErr != nil {
			return 0, fmt.Errorf("reseal %s row: %w", table.table, sealErr)
		}
		query = fmt.Sprintf("UPDATE %s SET %s=$2,key_id=$3 WHERE id=$1", table.table, table.column)
		if _, err = tx.Exec(ctx, query, id, sealed, active); err != nil {
			return 0, fmt.Errorf("write %s resealed row: %w", table.table, err)
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit key reseal: %w", err)
	}
	return id, nil
}
