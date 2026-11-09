package build

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/chunk"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"danny.vn/trachuan/internal/corpusdb"
	"danny.vn/trachuan/internal/embedding"
	"danny.vn/trachuan/internal/retrieval"
	"danny.vn/trachuan/internal/retrieval/lexical"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Process builds and dumps a corpus from the planned inventory.
func (s *Stage) Process(ctx context.Context, env stage.Env, run stage.Run, item stage.Item) (stage.Output, error) {
	started := time.Now()
	ctx, cancel := context.WithTimeout(ctx, 50*time.Minute)
	defer cancel()
	if s.cfg.Now == nil {
		return stage.Output{}, fmt.Errorf("missing build clock")
	}
	adminCfg, err := pgx.ParseConfig(s.cfg.BuilderURL)
	if err != nil {
		return stage.Output{}, fmt.Errorf("parse builder connection: %w", err)
	}
	adminCfg.Database = "postgres"
	admin, err := pgx.ConnectConfig(ctx, adminCfg)
	if err != nil {
		return stage.Output{}, fmt.Errorf("connect builder: %w", err)
	}
	defer func() { _ = admin.Close(context.Background()) }()
	if err = lock(ctx, admin, run.Country, run.Rel); err != nil {
		return stage.Output{}, err
	}
	defer func() { _ = unlock(context.Background(), admin, run.Country, run.Rel) }()
	if err = checkGeneration(ctx, env, run); err != nil {
		return stage.Output{}, err
	}
	planned, err := s.Plan(ctx, env, run)
	if err != nil {
		return stage.Output{}, err
	}
	if len(planned) != 1 || planned[0].Key != item.Key || planned[0].InputHash != item.InputHash {
		return stage.Output{}, fmt.Errorf("inventory_changed")
	}
	inv, err := LoadInventory(ctx, store.New(env.DB), run, "embed")
	if err != nil {
		return stage.Output{}, err
	}
	if stage.RecipeHash("build-input", "1", run.Country, run.Rel, inv.SourceManifestHash(), fmt.Sprint(corpusdb.SchemaVersion)) != item.InputHash {
		return stage.Output{}, fmt.Errorf("inventory_changed")
	}
	recipeHash := s.Recipe(run.Snapshot)
	if len(item.InputHash) != 64 || len(recipeHash) != 64 {
		return stage.Output{}, fmt.Errorf("invalid build hashes")
	}
	name := "corpus_tmp_" + run.Country + "_" + item.InputHash[:24]
	marker, err := encodeOwnershipMarker(ownershipMarker{RunKey: run.Country + "/" + run.Rel, InputHash: item.InputHash, RecipeHash: recipeHash, Instance: "built"})
	if err != nil {
		return stage.Output{}, err
	}
	if err = s.cfg.checkDisks(env.Artifacts.Root, s.cfg.beforeBytes()); err != nil {
		return stage.Output{}, err
	}
	if err = recreateTemporary(ctx, admin, s.cfg, name, marker); err != nil {
		return stage.Output{}, err
	}
	workCtx, cancelWork := context.WithCancel(ctx)
	defer cancelWork()
	stopWatch := watchLock(workCtx, admin, cancelWork, func() error { return s.cfg.checkDisks(env.Artifacts.Root, s.cfg.duringBytes()) })
	defer func() { _ = stopWatch() }()
	ctx = workCtx
	dbCfg, err := pgxpool.ParseConfig(s.cfg.BuilderURL)
	if err != nil {
		return stage.Output{}, fmt.Errorf("parse corpus connection: %w", err)
	}
	dbCfg.ConnConfig.Database = name
	pool, err := pgxpool.NewWithConfig(ctx, dbCfg)
	if err != nil {
		return stage.Output{}, fmt.Errorf("open corpus pool: %w", err)
	}
	defer pool.Close()
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return stage.Output{}, fmt.Errorf("acquire corpus connection: %w", err)
	}
	defer conn.Release()
	actualVersions, err := inspectVersions(ctx, s.cfg, conn.Conn())
	if err != nil {
		return stage.Output{}, err
	}
	if actualVersions != s.cfg.Versions {
		return stage.Output{}, fmt.Errorf("build runtime versions changed")
	}
	if err = corpusdb.ApplyTables(ctx, conn.Conn(), 1024); err != nil {
		return stage.Output{}, err
	}
	codes, err := source.Parse(run.Snapshot.SourceList)
	if err != nil {
		return stage.Output{}, fmt.Errorf("parse sources: %w", err)
	}
	counts, err := Load(ctx, conn.Conn(), inv, env.Artifacts, Recipes{Dense: embedding.DocumentRecipe1(), Lexical: lexical.Recipe1()}, codes.StatusCodes)
	if err != nil {
		return stage.Output{}, err
	}
	if _, err = conn.Exec(ctx, `SET maintenance_work_mem = '512MB'; SET max_parallel_maintenance_workers = 0`); err != nil {
		return stage.Output{}, fmt.Errorf("set index limits: %w", err)
	}
	if err = corpusdb.ApplyIndexes(ctx, conn.Conn()); err != nil {
		return stage.Output{}, err
	}
	if _, err = conn.Exec(ctx, `ANALYZE`); err != nil {
		return stage.Output{}, fmt.Errorf("analyze corpus: %w", err)
	}
	chunkJSON, _ := json.Marshal(chunk.Recipe1())
	doc, query := embedding.DocumentRecipe1(), embedding.QueryRecipe1()
	release := retrieval.Release{Country: run.Country, Rel: run.Rel, SchemaVersion: corpusdb.SchemaVersion, DenseRecipe: retrieval.DenseRecipe{Version: 1, Model: doc.Model, Dimensions: doc.Dimensions, Document: doc, DocumentHash: doc.Hash(), Query: query, QueryHash: query.Hash()}, ChunkRecipe: chunkJSON, LexicalRecipe: lexical.Recipe1(), RetrievalRecipe: retrieval.Recipe1(), LexicalStats: counts.LexicalStats, SourceManifestHash: inv.SourceManifestHash(), BuildInputHash: item.InputHash, BuildRecipeHash: recipeHash, EvaluationSummary: json.RawMessage(`{}`), BuiltAt: s.cfg.Now().UTC().Truncate(time.Microsecond)}
	cal, err := calibrate(ctx, s.cfg, pool, release)
	if err != nil {
		return stage.Output{}, err
	}
	calJSON, err := json.Marshal(cal)
	if err != nil {
		return stage.Output{}, fmt.Errorf("encode calibration: %w", err)
	}
	release.RetrievalRecipe.Abstention.Threshold = &cal.Threshold
	release.RetrievalRecipe.Abstention.CalibrationSHA256 = fmt.Sprintf("%x", sha256.Sum256(calJSON))
	if err = insertRelease(ctx, conn.Conn(), release); err != nil {
		return stage.Output{}, err
	}
	tableCounts, relationBytes, indexBytes, err := databaseMetrics(ctx, conn.Conn(), counts)
	if err != nil {
		return stage.Output{}, err
	}
	for reason, count := range exclusionCounts(inv) {
		tableCounts[reason] = count
	}
	if err = checkGeneration(ctx, env, run); err != nil {
		return stage.Output{}, err
	}
	final, err := DatabaseName(run.Country, run.Rel)
	if err != nil {
		return stage.Output{}, err
	}
	art, err := env.Artifacts.Publish(ctx, "build", item.InputHash, recipeHash, func(dir string) error {
		dumpPath := filepath.Join(dir, "corpus.dump")
		if dumpErr := pgDump(ctx, s.cfg, name, dumpPath); dumpErr != nil {
			return dumpErr
		}
		dumpFile, openErr := os.Open(dumpPath)
		if openErr != nil {
			return fmt.Errorf("open dump: %w", openErr)
		}
		hasher := sha256.New()
		size, copyErr := io.Copy(hasher, dumpFile)
		closeErr := dumpFile.Close()
		if copyErr != nil {
			return fmt.Errorf("hash dump: %w", copyErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close dump: %w", closeErr)
		}
		record := map[string]any{"version": 1, "country": run.Country, "rel": run.Rel, "database_name": final, "schema_version": corpusdb.SchemaVersion, "build_input_hash": item.InputHash, "build_recipe_hash": recipeHash, "source_manifest_hash": inv.SourceManifestHash(), "identity_inventory_hash": inv.IdentityHash(), "dense_recipe": release.DenseRecipe, "chunk_recipe": release.ChunkRecipe, "lexical_recipe": release.LexicalRecipe, "retrieval_recipe": release.RetrievalRecipe, "lexical_stats": release.LexicalStats, "taxonomy_version": "", "counts": tableCounts, "postgres_version": s.cfg.Versions.PostgreSQLMajor, "pgvector_version": s.cfg.Versions.PGVector, "ltree_version": s.cfg.Versions.Ltree, "pg_dump_version": s.cfg.Versions.PgDump, "worker_version": run.Snapshot.WorkerVersion, "durations_ms": map[string]int64{"total": time.Since(started).Milliseconds()}, "relation_bytes": relationBytes, "index_bytes": indexBytes, "dump": map[string]any{"sha256": fmt.Sprintf("%x", hasher.Sum(nil)), "bytes": size, "format": "custom"}, "built_at": release.BuiltAt}
		for _, f := range []struct {
			name  string
			value any
		}{{"inputs.json", inv}, {"build.json", record}, {"calibration.json", json.RawMessage(calJSON)}} {
			b, e := json.Marshal(f.value)
			if e != nil {
				return fmt.Errorf("encode %s: %w", f.name, e)
			}
			if e = os.WriteFile(filepath.Join(dir, f.name), b, 0600); e != nil {
				return fmt.Errorf("write %s: %w", f.name, e)
			}
		}
		if e := checkGeneration(ctx, env, run); e != nil {
			return e
		}
		return ctx.Err()
	})
	if err != nil {
		return stage.Output{}, fmt.Errorf("publish build: %w", err)
	}
	if err = stopWatch(); err != nil {
		return stage.Output{}, err
	}
	if err = admin.Ping(ctx); err != nil {
		return stage.Output{}, fmt.Errorf("build lock lost: %w", err)
	}
	return stage.Output{Artifact: &art, State: "done"}, nil
}

func checkGeneration(ctx context.Context, env stage.Env, run stage.Run) error {
	generation, err := store.New(env.DB).GetRunGeneration(ctx, run.ID)
	if err != nil {
		return fmt.Errorf("read run generation: %w", err)
	}
	if int(generation) != run.Generation {
		return stage.ErrSuperseded
	}
	return nil
}

func insertRelease(ctx context.Context, conn *pgx.Conn, r retrieval.Release) error {
	_, err := conn.Exec(ctx, `INSERT INTO release(singleton,country,rel,schema_version,taxonomy_version,dense_recipe,chunk_recipe,lexical_recipe,retrieval_recipe,lexical_stats,source_manifest_hash,build_input_hash,build_recipe_hash,evaluation_summary,built_at) VALUES(true,$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`, r.Country, r.Rel, r.SchemaVersion, r.TaxonomyVersion, r.DenseRecipe, r.ChunkRecipe, r.LexicalRecipe, r.RetrievalRecipe, r.LexicalStats, r.SourceManifestHash, r.BuildInputHash, r.BuildRecipeHash, r.EvaluationSummary, r.BuiltAt)
	if err != nil {
		return fmt.Errorf("insert release: %w", err)
	}
	return nil
}

// Rows provides no workshop projections for a build artifact.
func (*Stage) Rows(context.Context, stage.Env, artifact.Result) (func(context.Context, pgx.Tx, int64) error, error) {
	return nil, nil
}

// Resolve verifies the build artifact and ensures its corpus database exists.
func (s *Stage) Resolve(ctx context.Context, env stage.Env, run stage.Run, item stage.Item, art artifact.Result) (stage.Resolution, error) {
	if item.Key != run.Country+"/"+run.Rel || item.InputHash != art.InputHash {
		return stage.Resolution{}, fmt.Errorf("build item identity mismatch")
	}
	db, err := EnsureDatabase(ctx, s.cfg, env.Artifacts, art, run)
	if err != nil {
		return stage.Resolution{}, fmt.Errorf("resolve build database: %w", err)
	}
	gaps, err := buildGaps(ctx, s.cfg, db.Name)
	if err != nil {
		return stage.Resolution{}, err
	}
	return stage.Resolution{Gaps: gaps}, nil
}
