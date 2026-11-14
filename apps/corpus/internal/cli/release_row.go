package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"danny.vn/trachuan/apps/corpus/internal/stage/build"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"danny.vn/trachuan/internal/retrieval"
	"danny.vn/trachuan/internal/retrieval/lexical"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func runReleaseRow(ctx context.Context, args []string, _ io.Writer, stderr io.Writer) int {
	return runReleaseRowWith(ctx, args, stderr, func(ctx context.Context, cc, rel string) ([]byte, error) {
		return readReleaseRow(ctx, cc, rel, os.Getenv("CORPUS_BUILDER_DATABASE_URL"), os.Getenv("CORPUS_DATABASE_URL"))
	})
}

func runReleaseRowWith(ctx context.Context, args []string, stderr io.Writer, load func(context.Context, string, string) ([]byte, error)) int {
	fail := func(code int, err error) int { _, _ = fmt.Fprintln(stderr, err); return code }
	if len(args) < 2 {
		return fail(2, fmt.Errorf("usage: corpus release-row <cc> <rel> --out FILE"))
	}
	if _, err := build.DatabaseName(args[0], args[1]); err != nil {
		return fail(2, err)
	}
	opts, err := options(args[2:], map[string]bool{"--out": true})
	if err != nil {
		return fail(2, err)
	}
	if opts["--out"] == "" {
		return fail(2, fmt.Errorf("--out FILE is required"))
	}
	data, err := load(ctx, args[0], args[1])
	if err != nil {
		return fail(1, err)
	}
	file, err := os.OpenFile(opts["--out"], os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fail(1, fmt.Errorf("create release row output: %w", err))
	}
	_, writeErr := file.Write(append(data, '\n'))
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		_ = os.Remove(opts["--out"])
		if writeErr != nil {
			return fail(1, fmt.Errorf("write release row: %w", writeErr))
		}
		return fail(1, fmt.Errorf("close release row: %w", closeErr))
	}
	return 0
}

func releaseRowPoolConfig(url, database string) (*pgxpool.Config, error) {
	if url == "" {
		return nil, fmt.Errorf("release-row requires CORPUS_BUILDER_DATABASE_URL and CORPUS_DATABASE_URL")
	}
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("invalid release-row database configuration")
	}
	if database != "" {
		cfg.ConnConfig.Database = database
	}
	cfg.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
	return cfg, nil
}

func readReleaseRow(ctx context.Context, cc, rel, builderURL, workshopURL string) ([]byte, error) {
	name, err := build.DatabaseName(cc, rel)
	if err != nil {
		return nil, err
	}
	cfg, err := releaseRowPoolConfig(builderURL, name)
	if err != nil {
		return nil, err
	}
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("open release metadata failed")
	}
	defer db.Close()
	var raw []byte
	if err = db.QueryRow(ctx, `SELECT to_jsonb(r) - 'singleton' FROM release r WHERE singleton`).Scan(&raw); err != nil {
		return nil, fmt.Errorf("read release metadata failed")
	}
	row := map[string]json.RawMessage{}
	if err = json.Unmarshal(raw, &row); err != nil {
		return nil, fmt.Errorf("decode release metadata: %w", err)
	}
	var rowCC, rowRel string
	if json.Unmarshal(row["country"], &rowCC) != nil || json.Unmarshal(row["rel"], &rowRel) != nil || rowCC != cc || rowRel != rel {
		return nil, fmt.Errorf("release metadata identity mismatch")
	}
	extensions := map[string]string{}
	rows, err := db.Query(ctx, `SELECT extname, extversion FROM pg_extension WHERE extname <> 'plpgsql' ORDER BY extname`)
	if err != nil {
		return nil, fmt.Errorf("read release extensions failed")
	}
	defer rows.Close()
	for rows.Next() {
		var extension, version string
		if err = rows.Scan(&extension, &version); err != nil {
			return nil, fmt.Errorf("decode release extension: %w", err)
		}
		extensions[extension] = version
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("read release extensions failed")
	}
	inputs, err := releaseRecipeHashInputs(row)
	if err != nil {
		return nil, err
	}
	summary, err := readReleaseRun(ctx, cc, rel, workshopURL, row)
	if err != nil {
		return nil, err
	}
	for key, value := range map[string]any{"extensions": extensions, "run": summary, "recipe_hash_inputs": inputs} {
		row[key], err = json.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("encode release %s: %w", key, err)
		}
	}
	data, err := json.Marshal(row)
	if err != nil {
		return nil, fmt.Errorf("encode release row: %w", err)
	}
	return data, nil
}

func releaseRecipeHashInputs(row map[string]json.RawMessage) (map[string]string, error) {
	var dense retrieval.DenseRecipe
	var lex lexical.Recipe
	var recipe retrieval.Recipe
	for _, field := range []struct {
		key string
		out any
	}{{"dense_recipe", &dense}, {"lexical_recipe", &lex}, {"retrieval_recipe", &recipe}} {
		if err := json.Unmarshal(row[field.key], field.out); err != nil {
			return nil, fmt.Errorf("decode %s: %w", field.key, err)
		}
	}
	inputs := map[string]string{"chunk": string(row["chunk_recipe"])}
	for key, encode := range map[string]func() ([]byte, error){"query": dense.Query.CompactJSON, "document": dense.Document.CompactJSON, "lexical": lex.JSON, "retrieval": recipe.JSON} {
		data, err := encode()
		if err != nil {
			return nil, fmt.Errorf("encode %s hash input: %w", key, err)
		}
		inputs[key] = string(data)
	}
	return inputs, nil
}

type releaseRun struct {
	ID                  int64  `json:"id"`
	Complete            bool   `json:"complete"`
	ExpectedItems       int64  `json:"expected_items"`
	IncompleteItems     int64  `json:"incomplete_items"`
	UnacceptedGaps      int64  `json:"unaccepted_gaps"`
	BuildArtifactSHA256 string `json:"build_artifact_sha256"`
}

func readReleaseRun(ctx context.Context, cc, rel, url string, row map[string]json.RawMessage) (releaseRun, error) {
	cfg, err := releaseRowPoolConfig(url, "")
	if err != nil {
		return releaseRun{}, err
	}
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return releaseRun{}, fmt.Errorf("open release workshop failed")
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return releaseRun{}, fmt.Errorf("open release run snapshot failed")
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := store.New(tx)
	run, err := q.GetRun(ctx, store.GetRunParams{Country: cc, Rel: rel})
	if err != nil {
		return releaseRun{}, fmt.Errorf("read release run failed")
	}
	summary := releaseRun{ID: run.ID, Complete: run.State == "evaluated" || run.State == "released"}
	counts, err := q.ListRunItemCounts(ctx, run.ID)
	if err != nil {
		return releaseRun{}, fmt.Errorf("read release item counts failed")
	}
	for _, count := range counts {
		summary.ExpectedItems += count.Count
		if count.State != "done" && count.State != "gap" {
			summary.IncompleteItems += count.Count
		}
	}
	gaps, err := q.ListRunGaps(ctx, store.ListRunGapsParams{Country: cc, Rel: rel})
	if err != nil {
		return releaseRun{}, fmt.Errorf("read release gaps failed")
	}
	gapItems := map[string]bool{}
	for _, gap := range gaps {
		gapItems[gap.Stage+"\x00"+gap.ItemKey] = true
		if gap.AcceptedAt == nil {
			summary.UnacceptedGaps++
		}
	}
	raw, err := q.GetBuildInventory(ctx, run.ID)
	if err != nil {
		return releaseRun{}, fmt.Errorf("read release build inventory failed")
	}
	var inventory struct {
		Items []struct {
			Stage, State string
			Key          string `json:"item_key"`
			InputHash    string `json:"input_hash"`
			RecipeHash   string `json:"recipe_hash"`
			ArtifactHash string `json:"artifact_sha256"`
		} `json:"items"`
	}
	if err = json.Unmarshal(raw, &inventory); err != nil {
		return releaseRun{}, fmt.Errorf("decode release inventory: %w", err)
	}
	var inputHash, recipeHash string
	if json.Unmarshal(row["build_input_hash"], &inputHash) != nil || json.Unmarshal(row["build_recipe_hash"], &recipeHash) != nil {
		return releaseRun{}, fmt.Errorf("invalid release build hashes")
	}
	for _, item := range inventory.Items {
		if item.State == "gap" && !gapItems[item.Stage+"\x00"+item.Key] {
			summary.Complete = false
		}
		if item.Stage == "build" && item.Key == cc+"/"+rel && item.State == "done" && item.InputHash == inputHash && item.RecipeHash == recipeHash {
			summary.BuildArtifactSHA256 = item.ArtifactHash
		}
	}
	if summary.BuildArtifactSHA256 == "" {
		return releaseRun{}, fmt.Errorf("release does not match the run's done build")
	}
	summary.Complete = summary.Complete && summary.ExpectedItems > 0 && summary.IncompleteItems == 0 && summary.UnacceptedGaps == 0
	return summary, nil
}
