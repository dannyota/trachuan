package oauth

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"danny.vn/trachuan/apps/server/internal/store"
	"github.com/jackc/pgx/v5"
)

// ClientsLockKey is shared by admission and every transaction creating client
// authority. PostgreSQL releases these transaction locks only at commit/rollback.
const ClientsLockKey int64 = 0x747261636c69656e

// Admit takes the exclusive clients lock and admits a validated client. The
// caller commits the transaction; it must not first acquire the shared lock.
func Admit(ctx context.Context, tx pgx.Tx, client Client) (Client, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	unavailable := func(err error) (Client, error) {
		return Client{}, fmt.Errorf("admit OAuth client: %w: %w", ErrTemporarilyUnavailable, err)
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, ClientsLockKey); err != nil {
		return unavailable(err)
	}
	q := store.New(tx)
	existing, err := q.GetClient(ctx, client.ID)
	if err == nil {
		return clientFromRow(existing), nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return unavailable(err)
	}
	count, err := q.CountClients(ctx)
	if err != nil {
		return unavailable(err)
	}
	if count >= 10000 {
		if _, evictErr := q.EvictClient(ctx); evictErr != nil {
			return unavailable(evictErr)
		}
	}
	var digest *string
	if client.Kind == "cimd" {
		raw, marshalErr := json.Marshal(struct {
			Name      string
			Redirects []string
		}{client.Name, client.RedirectURIs})
		if marshalErr != nil {
			return unavailable(marshalErr)
		}
		value := fmt.Sprintf("%x", sha256.Sum256(raw))
		digest = &value
	}
	row, err := q.CreateClient(ctx, store.CreateClientParams{ClientID: client.ID, Kind: client.Kind, Name: client.Name, RedirectUris: client.RedirectURIs, MetadataSha256: digest})
	if err != nil {
		return unavailable(err)
	}
	return clientFromRow(row), nil
}

// InUse takes the shared clients lock, reads the client and touches its hourly
// usage timestamp. Call it in the same transaction before storing a pending
// request, code or token family. A missing row means the client is unknown.
func InUse(ctx context.Context, tx pgx.Tx, clientID string) (Client, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	unavailable := func(err error) (Client, error) {
		return Client{}, fmt.Errorf("protect OAuth client: %w: %w", ErrTemporarilyUnavailable, err)
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock_shared($1)`, ClientsLockKey); err != nil {
		return unavailable(err)
	}
	q := store.New(tx)
	row, err := q.GetClient(ctx, clientID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Client{}, ErrInvalidClient
	}
	if err != nil {
		return unavailable(err)
	}
	if _, err := q.TouchClient(ctx, clientID); err != nil {
		return unavailable(err)
	}
	return clientFromRow(row), nil
}
