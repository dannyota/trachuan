package build

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"danny.vn/trachuan/internal/pgdrop"
	"github.com/jackc/pgx/v5"
)

// Database identifies a verified corpus database.
type Database struct {
	Name, URL string
	Record    BuildRecord
	Instance  string
}

// EnsureDatabase resolves a verified artifact into a local corpus database.
func EnsureDatabase(ctx context.Context, cfg Config, artifacts artifact.Store, art artifact.Result, run stage.Run) (Database, error) {
	record, dump, err := verifyBuildArtifact(artifacts, art, run, cfg)
	if err != nil {
		return Database{}, err
	}
	adminCfg, err := pgx.ParseConfig(cfg.BuilderURL)
	if err != nil {
		return Database{}, fmt.Errorf("parse builder connection: %w", err)
	}
	adminCfg.Database = "postgres"
	admin, err := pgx.ConnectConfig(ctx, adminCfg)
	if err != nil {
		return Database{}, fmt.Errorf("connect builder: %w", err)
	}
	defer func() { _ = admin.Close(context.Background()) }()
	if err = lock(ctx, admin, run.Country, run.Rel); err != nil {
		return Database{}, err
	}
	defer func() { _ = unlock(context.Background(), admin, run.Country, run.Rel) }()
	temporary := "corpus_tmp_" + run.Country + "_" + record.BuildInputHash[:24]
	marker, err := encodeOwnershipMarker(ownershipMarker{RunKey: run.Country + "/" + run.Rel, InputHash: record.BuildInputHash, RecipeHash: record.BuildRecipeHash, Instance: "built"})
	if err != nil {
		return Database{}, err
	}
	exists, owned, err := databaseOwnership(ctx, admin, record.DatabaseName)
	if err != nil {
		return Database{}, err
	}
	instance := "built"
	if exists {
		found, markerErr := readMarker(ctx, admin, record.DatabaseName)
		if markerErr != nil || !owned || !sameOwnership(found, marker) {
			return Database{}, fmt.Errorf("final database ownership mismatch")
		}
		if err = validateLocked(ctx, admin, cfg, record.DatabaseName, record); err != nil {
			return Database{}, fmt.Errorf("validate final database: %w", err)
		}
		instance, err = databaseInstance(ctx, admin, record.DatabaseName)
		if err != nil {
			return Database{}, err
		}
	} else {
		exists, owned, err = databaseOwnership(ctx, admin, temporary)
		if err != nil {
			return Database{}, err
		}
		valid := false
		if exists {
			found, markerErr := readMarker(ctx, admin, temporary)
			if markerErr != nil {
				return Database{}, markerErr
			}
			if !owned || (found != "" && !sameOwnership(found, marker)) {
				return Database{}, fmt.Errorf("temporary database ownership mismatch")
			}
			if sameOwnership(found, marker) {
				validationErr := validateLocked(ctx, admin, cfg, temporary, record)
				if validationErr != nil && !incompleteDatabase(validationErr) {
					return Database{}, fmt.Errorf("inspect temporary database: %w", validationErr)
				}
				valid = validationErr == nil
				if valid {
					instance, err = databaseInstance(ctx, admin, temporary)
					if err != nil {
						return Database{}, err
					}
				}
			}
			if !valid {
				if err = cfg.checkDisks(artifacts.Root, cfg.beforeBytes()); err != nil {
					return Database{}, err
				}
				var active bool
				if err = admin.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=$1 AND usesysid IS NOT NULL)`, temporary).Scan(&active); err != nil {
					return Database{}, fmt.Errorf("inspect database sessions: %w", err)
				}
				if active {
					return Database{}, fmt.Errorf("temporary database has active sessions")
				}
				if err = pgdrop.DropOnly(ctx, admin, temporary); err != nil {
					return Database{}, fmt.Errorf("drop temporary database: %w", err)
				}
			}
		}
		if !valid {
			if err = cfg.checkDisks(artifacts.Root, cfg.beforeBytes()); err != nil {
				return Database{}, err
			}
			if err = recreateTemporary(ctx, admin, cfg, temporary, marker); err != nil {
				return Database{}, err
			}
			work, cancel := context.WithCancel(ctx)
			stop := watchLock(work, admin, cancel, func() error { return cfg.checkDisks(artifacts.Root, cfg.duringBytes()) })
			restoreErr := pgRestore(work, cfg, temporary, dump)
			watchErr := stop()
			cancel()
			if restoreErr != nil {
				return Database{}, restoreErr
			}
			if watchErr != nil {
				return Database{}, watchErr
			}
			if err = validateLocked(ctx, admin, cfg, temporary, record); err != nil {
				return Database{}, fmt.Errorf("validate restored database: %w", err)
			}
			if err = markRestored(ctx, admin, temporary); err != nil {
				return Database{}, err
			}
			instance = "restored"
		}
		if err = admin.Ping(ctx); err != nil {
			return Database{}, fmt.Errorf("build lock lost: %w", err)
		}
		if err = renameToFinal(ctx, admin, temporary, record.DatabaseName); err != nil {
			return Database{}, err
		}
	}
	if err = admin.Ping(ctx); err != nil {
		return Database{}, fmt.Errorf("build lock lost: %w", err)
	}
	u, err := url.Parse(cfg.BuilderURL)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") {
		return Database{}, fmt.Errorf("invalid builder URL")
	}
	u.Path = "/" + record.DatabaseName
	return Database{Name: record.DatabaseName, URL: u.String(), Record: record, Instance: instance}, nil
}

func databaseOwnership(ctx context.Context, conn *pgx.Conn, name string) (bool, bool, error) {
	var exists, owned bool
	err := conn.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname=$1), EXISTS(SELECT 1 FROM pg_database WHERE datname=$1 AND datdba=(SELECT oid FROM pg_roles WHERE rolname=current_user))`, name).Scan(&exists, &owned)
	if err != nil {
		return false, false, fmt.Errorf("inspect database ownership: %w", err)
	}
	return exists, owned, nil
}

// VerifyForRun finds and verifies the committed build artifact without opening a corpus database.
func VerifyForRun(ctx context.Context, cfg Config, q store.Querier, artifacts artifact.Store, run stage.Run) (artifact.Result, error) {
	return committedBuild(ctx, cfg, q, artifacts, run)
}

func committedBuild(ctx context.Context, cfg Config, q store.Querier, artifacts artifact.Store, run stage.Run) (artifact.Result, error) {
	raw, err := q.GetBuildInventory(ctx, run.ID)
	if err != nil {
		return artifact.Result{}, fmt.Errorf("read build item: %w", err)
	}
	var paths struct {
		Items []struct {
			Stage          string  `json:"stage"`
			ItemKey        string  `json:"item_key"`
			State          string  `json:"state"`
			Path           *string `json:"path"`
			InputHash      *string `json:"input_hash"`
			RecipeHash     *string `json:"recipe_hash"`
			ArtifactSHA256 *string `json:"artifact_sha256"`
		} `json:"items"`
	}
	if err = json.Unmarshal(raw, &paths); err != nil {
		return artifact.Result{}, fmt.Errorf("decode build artifact: %w", err)
	}

	for _, item := range paths.Items {
		if item.Stage != "build" || item.ItemKey != run.Country+"/"+run.Rel || item.State != "done" {
			continue
		}
		if item.Path == nil || item.InputHash == nil || item.RecipeHash == nil || item.ArtifactSHA256 == nil {
			return artifact.Result{}, errors.New("invalid committed build artifact")
		}
		art := artifact.Result{Path: *item.Path, SHA256: *item.ArtifactSHA256, InputHash: *item.InputHash}
		record, _, err := verifyBuildArtifact(artifacts, art, run, cfg)
		if err != nil {
			return artifact.Result{}, fmt.Errorf("verify committed build artifact: %w", err)
		}
		if !New(cfg).acceptsRecipeHash(run.Snapshot, *item.RecipeHash) || record.BuildRecipeHash != *item.RecipeHash {
			return artifact.Result{}, errors.New("invalid committed build artifact")
		}
		return art, nil
	}
	return artifact.Result{}, errors.New("completed build artifact not found")
}

// EnsureForRun finds the committed build artifact and resolves its database.
func EnsureForRun(ctx context.Context, cfg Config, q store.Querier, artifacts artifact.Store, run stage.Run) (Database, error) {
	db, _, err := EnsureForRunWithArtifact(ctx, cfg, q, artifacts, run)
	return db, err
}

// EnsureForRunWithArtifact resolves the committed build and returns its verified artifact.
func EnsureForRunWithArtifact(ctx context.Context, cfg Config, q store.Querier, artifacts artifact.Store, run stage.Run) (Database, artifact.Result, error) {
	art, err := committedBuild(ctx, cfg, q, artifacts, run)
	if err != nil {
		return Database{}, artifact.Result{}, err
	}
	db, err := EnsureDatabase(ctx, cfg, artifacts, art, run)
	if err != nil {
		return Database{}, artifact.Result{}, err
	}
	return db, art, nil
}
