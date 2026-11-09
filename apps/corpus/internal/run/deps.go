package run

import (
	"encoding/json"
	"time"

	"danny.vn/traquydinh/apps/corpus/internal/stage"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.temporal.io/sdk/client"
)

// Deps supplies the workshop, workflow client, and compiled release inputs.
type Deps struct {
	BatchesInFlight int
	DB              *pgxpool.Pool
	Temporal        client.Client
	RecipesFor      func(stage.Snapshot) map[string]string
	Stages          []string
	SourceList      json.RawMessage
	Now             func() time.Time
}
