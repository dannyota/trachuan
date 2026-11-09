package run

import (
	"encoding/json"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.temporal.io/sdk/client"
)

// Deps supplies the workshop, workflow client, and compiled release inputs.
type Deps struct {
	Artifacts        artifact.Store
	TaskQueue        string
	BatchesInFlight  int
	HeartbeatTimeout time.Duration
	DB               *pgxpool.Pool
	Temporal         client.Client
	RecipesFor       func(stage.Snapshot) map[string]string
	Stages           []string
	SourceList       json.RawMessage
	Now              func() time.Time
}
