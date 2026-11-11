package oauth

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
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
	if client.Kind == "dcr" || client.Kind == "preregistered" {
		client.grants = grantAuthorizationCode | grantRefreshToken
	}
	grants := client.grantTypes()
	var digest *string
	if client.Kind == "cimd" {
		raw, marshalErr := json.Marshal(struct {
			Name      string
			Redirects []string
			Grants    uint8
		}{client.Name, client.RedirectURIs, client.grants})
		if marshalErr != nil {
			return unavailable(marshalErr)
		}
		value := fmt.Sprintf("%x", sha256.Sum256(raw))
		digest = &value
	}
	existing, err := q.GetClient(ctx, client.ID)
	if err == nil {
		sameDigest := existing.MetadataSha256 == nil && digest == nil || existing.MetadataSha256 != nil && digest != nil && *existing.MetadataSha256 == *digest
		if existing.Kind != client.Kind || existing.Name != client.Name || !slices.Equal(existing.RedirectUris, client.RedirectURIs) || !slices.Equal(existing.GrantTypes, grants) || !sameDigest {
			if _, updateErr := tx.Exec(ctx, `UPDATE oauth_client SET kind=$2, name=$3, redirect_uris=$4, metadata_sha256=$5, grant_types=$6 WHERE client_id=$1`, client.ID, client.Kind, client.Name, client.RedirectURIs, digest, grants); updateErr != nil {
				return unavailable(updateErr)
			}
			existing.Kind, existing.Name, existing.RedirectUris, existing.MetadataSha256, existing.GrantTypes = client.Kind, client.Name, client.RedirectURIs, digest, grants
		}
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
	row, err := q.CreateClient(ctx, store.CreateClientParams{ClientID: client.ID, Kind: client.Kind, Name: client.Name, RedirectUris: client.RedirectURIs, MetadataSha256: digest, GrantTypes: grants})
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
