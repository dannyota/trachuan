package build

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/evaluation"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"danny.vn/trachuan/internal/corpusdb"
	"danny.vn/trachuan/internal/retrieval"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Config supplies the resources used to build a corpus database.
type Config struct {
	BuilderURL, Template, PgDump, PgRestore string
	// DatabaseDiskPath identifies an existing directory on the database volume.
	DatabaseDiskPath string
	// MinFreeBeforeBytes defaults to 30 GiB when zero.
	MinFreeBeforeBytes uint64
	// MinFreeDuringBytes defaults to 10 GiB when zero.
	MinFreeDuringBytes uint64
	diskCheck          func(string, uint64) error
	Embedder           retrieval.Embedder
	Suite              evaluation.Suite
	Now                func() time.Time
	Versions           Versions
	Calibrate          func(context.Context, *pgxpool.Pool, retrieval.Release) (evaluation.Calibration, error)
}

// Stage builds one corpus database per release.
type Stage struct{ cfg Config }

// New constructs a build stage.
func New(cfg Config) *Stage { return &Stage{cfg: cfg} }

// Plan selects a single build after all predecessor items have finished.
func (s *Stage) Plan(ctx context.Context, env stage.Env, run stage.Run) ([]stage.Item, error) {
	if _, err := DatabaseName(run.Country, run.Rel); err != nil {
		return nil, err
	}
	date, err := time.Parse("20060102", run.Rel[:8])
	if err != nil {
		return nil, fmt.Errorf("invalid release date: %w", err)
	}
	if date.Year() == 0 {
		return nil, fmt.Errorf("invalid release date")
	}
	inv, err := LoadInventory(ctx, store.New(env.DB), run, "embed")
	if err != nil {
		return nil, err
	}
	if err = reconcileMembership(inv); err != nil {
		return nil, err
	}
	for _, item := range inv.Items {
		if item.State != "done" && item.State != "gap" {
			return nil, fmt.Errorf("predecessor %s is %s", item.Stage, item.State)
		}
	}
	identities, err := IdentityInventory(inv, env.Artifacts)
	if err != nil {
		return nil, fmt.Errorf("load identity inventory: %w", err)
	}
	mapping, err := s.cfg.Suite.PinnedMap()
	if err != nil {
		return nil, fmt.Errorf("verify identity map: %w", err)
	}
	if _, err = mapping.Validate(s.cfg.Suite.Legacy, s.cfg.Suite.Hashes.Legacy, identities); err != nil {
		return nil, fmt.Errorf("validate identity map: %w", err)
	}
	hash := inventoryInputHash(run.Country, run.Rel, inv, s.cfg.Suite.Hashes.IdentityMap)
	return []stage.Item{{Key: run.Country + "/" + run.Rel, InputHash: hash}}, nil
}

func buildInputHash(country, rel, manifest, identityMap string) string {
	return stage.RecipeHash("build-input", "2", country, rel, manifest, strconv.Itoa(corpusdb.SchemaVersion), identityMap)
}

func inventoryInputHash(country, rel string, inv Inventory, identityMap string) string {
	if inv.Version == 2 {
		return stage.RecipeHash("build-input", "3", country, rel, inv.SourceManifestHash(), strconv.Itoa(corpusdb.SchemaVersion), identityMap, hashJSON(inv.Provenance))
	}
	return buildInputHash(country, rel, inv.SourceManifestHash(), identityMap)
}
