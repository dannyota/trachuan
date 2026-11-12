package ocrcheck

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Lock holds a database-scoped checker lock until the invocation ends.
func (d *Database) Lock(ctx context.Context, checker string) (func(), error) {
	conn, err := d.pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("acquire OCR check lock connection: %w", err)
	}
	hash := sha256.Sum256([]byte("trachuan/ocr-check/lock/" + checker))
	key := int64(binary.BigEndian.Uint64(hash[:8]))
	var acquired bool
	err = conn.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", key).Scan(&acquired)
	if err != nil {
		closeLockConnection(conn.Hijack())
		return nil, fmt.Errorf("acquire OCR check lock: %w", err)
	}
	if !acquired {
		conn.Release()
		return nil, fmt.Errorf("another OCR check for this checker is running")
	}
	return func() {
		releaseCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		var released bool
		if err := conn.QueryRow(releaseCtx, "SELECT pg_advisory_unlock($1)", key).Scan(&released); err != nil || !released {
			closeLockConnection(conn.Hijack())
			return
		}
		conn.Release()
	}, nil
}

func closeLockConnection(conn *pgx.Conn) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = conn.Close(ctx)
}
