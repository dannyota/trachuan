package evaluate

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"

	"danny.vn/trachuan/internal/frameworks/build"
	"danny.vn/trachuan/internal/frameworks/serve"
	"github.com/jackc/pgx/v5"
)

func Restored(ctx context.Context, req EvaluationRequest, restore Restore, open Opener, publish func(EvaluationResult) error) error {
	var result EvaluationResult
	if restore == nil || open == nil {
		return fmt.Errorf("evaluation dependencies unavailable")
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return hide("create evaluation cursor key", err)
	}
	err := restore(ctx, req.DatabaseRequest, "_eval", false, func(ctx context.Context, conn *pgx.Conn) error {
		if err := verifyRestored(ctx, conn, req); err != nil {
			return err
		}
		result.Restored = true
		gaps, err := evaluationGaps(ctx, conn)
		if err != nil {
			return err
		}
		result.Gaps = gaps
		engine, closeEngine, err := open(ctx, conn, key)
		if err != nil {
			return hide("open evaluation engine", err)
		}
		defer closeEngine()
		status, err := engine.CorpusStatus(ctx, serve.StatusRequest{}, fullCaller())
		if err != nil {
			return hide("read evaluation status", err)
		}
		result.RetrievalRecipeStatus = status.RetrievalRecipeStatus
		result.Cases = scoreCases(ctx, engine, req)
		versions := map[string]int{}
		for handle := range status.Versions {
			versions[versionCode(handle)]++
		}
		result.Cohorts = map[string]CohortMetrics{}
		for _, cohort := range []string{"public", "licensed"} {
			result.Cohorts[cohort] = build.ComputeMetrics(result.Cases, cohort, versions)
		}
		req.ProbeTargets, err = mappingProbeTargets(ctx, conn)
		if err != nil {
			return err
		}
		result.Invariant = checkInvariant(ctx, req, engine, invariantCopies{restore: restore, open: open, mutate: mutateClass}, key, status)
		if err := ctx.Err(); err != nil {
			return err
		}
		return publish(result)
	})
	if err != nil {
		return hide("evaluate restored database", err)
	}
	return nil
}

func verifyRestored(ctx context.Context, conn *pgx.Conn, req EvaluationRequest) error {
	var rel, kind string
	var sealed bool
	var frameworkCount, versionCount, controlCount int
	var actualFrameworks, actualVersions, actualControls int
	var recipes [5]json.RawMessage
	var inputs [5]string
	var retrievalRecipe json.RawMessage
	err := conn.QueryRow(ctx, `SELECT rel,kind,versions_sealed,framework_count,version_count,control_count,
 (SELECT count(*) FROM framework),(SELECT count(*) FROM framework_version),(SELECT count(*) FROM control),
 snapshot_recipe,parse_recipe,relate_recipe,chunk_recipe,embed_recipe,
 snapshot_input_hash,parse_input_hash,relate_input_hash,chunk_input_hash,embed_input_hash,retrieval_recipe FROM release WHERE singleton`).Scan(
		&rel, &kind, &sealed, &frameworkCount, &versionCount, &controlCount, &actualFrameworks, &actualVersions, &actualControls,
		&recipes[0], &recipes[1], &recipes[2], &recipes[3], &recipes[4], &inputs[0], &inputs[1], &inputs[2], &inputs[3], &inputs[4], &retrievalRecipe)
	if err != nil {
		return hide("verify restored release", err)
	}
	if rel != req.Rel || kind != "frameworks" || !sealed || frameworkCount != actualFrameworks || versionCount != actualVersions || controlCount != actualControls {
		return fmt.Errorf("restored release mismatch")
	}
	for i, name := range []string{"snapshot", "parse", "relate", "chunk", "embed"} {
		var value any
		if json.Unmarshal(recipes[i], &value) != nil {
			return fmt.Errorf("invalid restored recipe")
		}
		if build.RecipeSHA256(value) != req.RecipeHashes[name] || inputs[i] != req.InputHashes[name] {
			return fmt.Errorf("restored provenance mismatch")
		}
	}
	expected, _ := json.Marshal(req.RetrievalRecipe)
	if !equalJSON(retrievalRecipe, expected) {
		return fmt.Errorf("restored retrieval recipe mismatch")
	}
	var violations int
	if err = conn.QueryRow(ctx, `SELECT (SELECT count(*) FROM control c LEFT JOIN control_text t ON t.control_id=c.id WHERE t.control_id IS NULL)`).Scan(&violations); err != nil {
		return hide("verify restored records", err)
	}
	if violations != 0 {
		return fmt.Errorf("incomplete restored records")
	}
	return nil
}
func evaluationGaps(ctx context.Context, conn *pgx.Conn) ([]GapTotal, error) {
	rows, err := conn.Query(ctx, `SELECT kind,class,blocks,sum(split_part(detail,':',1)::bigint) FROM quality_gap GROUP BY kind,class,blocks ORDER BY kind,class,blocks`)
	if err != nil {
		return nil, hide("read evaluation gap totals", err)
	}
	defer rows.Close()
	totals := []GapTotal{}
	for rows.Next() {
		var total GapTotal
		if err = rows.Scan(&total.Kind, &total.Class, &total.Blocks, &total.Count); err != nil {
			return nil, hide("read evaluation gap totals", err)
		}
		totals = append(totals, total)
	}
	if err = rows.Err(); err != nil {
		return nil, hide("read evaluation gap totals", err)
	}
	return totals, nil
}
