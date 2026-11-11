package stage

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"github.com/jackc/pgx/v5"
)

// AcquisitionMessage identifies an item committed by same-run acquisition reuse.
const AcquisitionMessage = "acquisition reused"

type acquisitionVerifier interface {
	VerifyAcquisition(context.Context, Env, Run, Item, artifact.Result) error
}

func adoptAcquisition(ctx context.Context, env Env, st Stage, r Run, item Item) (Output, bool, error) {
	verifier, ok := st.(acquisitionVerifier)
	if !ok || (st.Name() != "fetch" && st.Name() != "expand") || item.Skip != nil {
		return Output{}, false, nil
	}
	a, err := store.New(env.DB).GetRunAcquisition(ctx, store.GetRunAcquisitionParams{RunID: r.ID, Stage: st.Name(), DocumentKey: item.Key})
	if errors.Is(err, pgx.ErrNoRows) {
		return Output{}, false, nil
	}
	if err != nil {
		return Output{}, false, fmt.Errorf("read acquisition: %w", err)
	}
	if int(a.Generation) >= r.Generation {
		return Output{}, false, nil
	}
	miss := func(reason string) (Output, bool, error) {
		slog.Warn("acquisition unavailable", "run", r.Country+"/"+r.Rel, "stage", st.Name(), "key", item.Key, "reason", reason)
		return Output{}, false, nil
	}
	recipe := st.Recipe(r.Snapshot)
	if a.ArtifactStage != st.Name() || a.RecipeHash != recipe || r.Snapshot.Recipes[st.Name()] != recipe || a.InputHash != a.Sha256 {
		return miss("stage, frozen recipe or input hash mismatch")
	}
	art, found, err := env.Artifacts.OpenRecorded(st.Name(), a.InputHash, recipe, a.Sha256)
	if err != nil {
		return Output{}, false, err
	}
	if !found {
		return miss("bundle missing or invalid, or recorded manifest hash differs")
	}
	if filepath.Clean(env.Artifacts.Abs(a.Path)) != filepath.Clean(env.Artifacts.Abs(art.Path)) {
		return miss("recorded path mismatch")
	}
	if err = verifier.VerifyAcquisition(ctx, env, r, item, art); err != nil {
		var pathErr *os.PathError
		if ctx.Err() != nil || errors.As(err, &pathErr) && !errors.Is(err, os.ErrNotExist) {
			return Output{}, false, fmt.Errorf("read acquisition identity: %w", err)
		}
		return miss(err.Error())
	}
	parts := strings.SplitN(item.Key, ":", 3)
	if len(parts) != 3 || parts[0] != r.Country || parts[1] == "" || parts[2] == "" {
		return miss("document key mismatch")
	}
	return Output{State: "done", Message: AcquisitionMessage, Artifact: &art, Sources: []string{parts[1]}}, true, nil
}
