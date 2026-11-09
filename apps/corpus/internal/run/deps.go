package run

import (
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.temporal.io/sdk/client"
)

// Deps supplies the workshop, workflow client, and compiled release inputs.
type Deps struct {
	DB         *pgxpool.Pool
	Temporal   client.Client
	Recipes    map[string]string
	Stages     []string
	SourceList json.RawMessage
	Now        func() time.Time
}
