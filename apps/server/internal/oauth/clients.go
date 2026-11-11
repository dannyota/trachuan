package oauth

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"danny.vn/trachuan/apps/server/internal/store"
	"github.com/jackc/pgx/v5"
)

// ErrInvalidClient indicates an unknown client or invalid public metadata.
var ErrInvalidClient = errors.New("oauth client invalid")

// ErrTemporarilyUnavailable indicates a failed admission or auth database call.
var ErrTemporarilyUnavailable = errors.New("oauth temporarily unavailable")

// Client is validated public client metadata.
type Client struct {
	ID           string
	Kind         string
	Name         string
	RedirectURIs []string
	CreatedAt    time.Time
	grants       uint8
}

const (
	grantAuthorizationCode uint8 = 1 << iota
	grantRefreshToken
)

// Allows reports whether validated client metadata permits a grant. CIMD
// clients must come from Resolve or Admit, since InUse reads no grant metadata.
func (c Client) Allows(grant string) bool {
	var flag uint8
	switch grant {
	case "authorization_code":
		flag = grantAuthorizationCode
	case "refresh_token":
		flag = grantRefreshToken
	default:
		return false
	}
	if c.Kind == "dcr" || c.Kind == "preregistered" {
		return true
	}
	return c.Kind == "cimd" && c.grants&flag != 0
}

// ClientDB is the auth database used to resolve and admit clients.
type ClientDB interface {
	store.DBTX
	Begin(context.Context) (pgx.Tx, error)
}

// Clients resolves CIMD, pre-registered and dynamically registered clients.
type Clients struct {
	db            ClientDB
	preregistered map[string]Client
	cimd          *CIMDResolver
}

// NewClients copies startup configuration. CIMD always takes precedence.
func NewClients(db ClientDB, preregistered []Client, cimd *CIMDResolver) *Clients {
	c := &Clients{db: db, preregistered: map[string]Client{}, cimd: cimd}
	for _, client := range preregistered {
		c.preregistered[client.ID] = cloneClient(client)
	}
	return c
}

// Resolve returns current metadata and ensures configured clients are stored.
func (c *Clients) Resolve(ctx context.Context, id string) (Client, error) {
	if strings.HasPrefix(id, "https://") {
		if c.cimd == nil {
			return Client{}, ErrInvalidClient
		}
		client, err := c.cimd.Resolve(ctx, id)
		if err != nil {
			return Client{}, err
		}
		stored, err := c.Register(ctx, client)
		if err != nil {
			return Client{}, err
		}
		client.CreatedAt = stored.CreatedAt
		return client, nil
	}
	if client, ok := c.preregistered[id]; ok {
		stored, err := c.Register(ctx, client)
		if err != nil {
			return Client{}, err
		}
		client = cloneClient(client)
		client.CreatedAt = stored.CreatedAt
		return client, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	row, err := store.New(c.db).GetClient(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Client{}, ErrInvalidClient
	}
	if err != nil {
		return Client{}, fmt.Errorf("read OAuth client: %w: %w", ErrTemporarilyUnavailable, err)
	}
	if row.Kind != "dcr" {
		return Client{}, ErrInvalidClient
	}
	return clientFromRow(row), nil
}

// Register commits an admission. A failed commit never returns a usable client.
func (c *Clients) Register(ctx context.Context, client Client) (Client, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	tx, err := c.db.Begin(ctx)
	if err != nil {
		return Client{}, fmt.Errorf("begin client admission: %w: %w", ErrTemporarilyUnavailable, err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	admitted, err := Admit(ctx, tx, client)
	if err != nil {
		return Client{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Client{}, fmt.Errorf("commit client admission: %w: %w", ErrTemporarilyUnavailable, err)
	}
	return admitted, nil
}

func clientFromRow(row store.OauthClient) Client {
	return Client{ID: row.ClientID, Kind: row.Kind, Name: row.Name, RedirectURIs: slices.Clone(row.RedirectUris), CreatedAt: row.CreatedAt}
}
