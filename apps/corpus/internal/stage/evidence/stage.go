package evidence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/detector"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/build"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const serialization = "findings.json/1"

type Config struct {
	Build     build.Config
	Detectors []Detector
}

type Stage struct {
	cfg     Config
	queries func(stage.Env) store.Querier
	lookup  func(context.Context, stage.Env, store.Querier, stage.Run) (artifact.Result, error)
	release func(context.Context, stage.Env, stage.Run) (*pgxpool.Pool, func(), error)
}

func New(cfg Config) *Stage { return &Stage{cfg: cfg} }

func (*Stage) Name() string { return stage.EvidenceStage }

func (s *Stage) specs() []detector.Detector {
	specs := make([]detector.Detector, 0, len(s.cfg.Detectors))
	for _, d := range s.cfg.Detectors {
		specs = append(specs, d.Spec())
	}
	slices.SortFunc(specs, func(a, b detector.Detector) int { return strings.Compare(a.ID, b.ID) })
	return specs
}

func (s *Stage) Recipe(stage.Snapshot) string {
	config, err := json.Marshal(s.specs())
	if err != nil {
		panic(fmt.Sprintf("encode evidence detectors: %v", err))
	}
	return stage.RecipeHash(stage.EvidenceStage, "1", detector.Hash(), string(config), serialization)
}

func (s *Stage) querier(env stage.Env) store.Querier {
	if s.queries != nil {
		return s.queries(env)
	}
	return store.New(env.DB)
}

func (s *Stage) lookupBuild(ctx context.Context, env stage.Env, q store.Querier, run stage.Run) (artifact.Result, error) {
	if s.lookup != nil {
		return s.lookup(ctx, env, q, run)
	}
	return build.VerifyForRun(ctx, s.cfg.Build, q, env.Artifacts, run)
}

func itemKey(run stage.Run) string { return run.Country + "/" + run.Rel }

func (s *Stage) Plan(ctx context.Context, env stage.Env, run stage.Run) ([]stage.Item, error) {
	inputs, err := s.resolveInputs(ctx, env, run, true)
	var refused *inputError
	switch {
	case errors.As(err, &refused):
		return []stage.Item{{Key: itemKey(run), InputHash: stage.RecipeHash("evidence-input-refused", "1", refused.reason)}}, nil
	case err != nil:
		return nil, err
	}
	return []stage.Item{{Key: itemKey(run), InputHash: inputs.inputHash()}}, nil
}

func itemFailure(err error) error {
	var refused *inputError
	var incomplete *IncompleteError
	if errors.As(err, &refused) || errors.As(err, &incomplete) {
		return errors.New(err.Error())
	}
	return stage.Retryable(errors.New(err.Error()))
}

func (s *Stage) Process(ctx context.Context, env stage.Env, run stage.Run, item stage.Item) (stage.Output, error) {
	if item.Key != itemKey(run) {
		return stage.Output{}, errors.New("evidence item identity mismatch")
	}
	inputs, err := s.resolveInputs(ctx, env, run, false)
	if err != nil {
		return stage.Output{}, itemFailure(err)
	}
	decisions, err := reviewDecisions(inputs.knowledge)
	if err != nil {
		return stage.Output{}, itemFailure(err)
	}
	runs, err := s.runDetectors(ctx, env, run, inputs)
	if err != nil {
		return stage.Output{}, itemFailure(err)
	}
	file, err := compute(computeInput{Rel: run.Rel, Bindings: inputs.bindings(run.Rel), Runs: runs, Previous: inputs.predecessor.file, Decisions: decisions})
	if err != nil {
		return stage.Output{}, itemFailure(err)
	}
	data, err := file.encode()
	if err != nil {
		return stage.Output{}, itemFailure(err)
	}
	result, err := env.Artifacts.Publish(ctx, stage.EvidenceStage, inputs.inputHash(), s.Recipe(run.Snapshot), func(dir string) error {
		return os.WriteFile(filepath.Join(dir, findingsFile), data, 0600)
	})
	if err != nil {
		return stage.Output{}, itemFailure(fmt.Errorf("publish evidence: %w", err))
	}
	return stage.Output{Artifact: &result, State: "done"}, nil
}

func (s *Stage) runDetectors(ctx context.Context, env stage.Env, run stage.Run, inputs frozenInputs) ([]detectorRun, error) {
	if len(s.cfg.Detectors) == 0 {
		return nil, nil
	}
	detectors := slices.Clone(s.cfg.Detectors)
	slices.SortFunc(detectors, func(a, b Detector) int { return strings.Compare(a.Spec().ID, b.Spec().ID) })
	ids := make([]string, 0, len(detectors))
	for i, d := range detectors {
		id := d.Spec().ID
		if id == "" || (i > 0 && ids[i-1] == id) {
			return nil, &IncompleteError{Detector: id, Reason: "detector is not uniquely named"}
		}
		ids = append(ids, id)
	}
	pool, closePool, err := s.openRelease(ctx, env, run)
	if err != nil {
		return nil, &IncompleteError{Detector: strings.Join(ids, ","), Reason: "release database unavailable: " + err.Error()}
	}
	defer closePool()
	in := Inputs{Run: run, BuildDir: inputs.buildDir, ReportPath: inputs.reportPath, Release: pool}
	runs := make([]detectorRun, 0, len(detectors))
	for _, d := range detectors {
		observation, detectErr := d.Detect(ctx, in)
		if detectErr != nil {
			return nil, &IncompleteError{Detector: d.Spec().ID, Reason: detectErr.Error()}
		}
		runs = append(runs, detectorRun{spec: d.Spec(), observation: observation})
	}
	return runs, nil
}

func (s *Stage) openRelease(ctx context.Context, env stage.Env, run stage.Run) (*pgxpool.Pool, func(), error) {
	if s.release != nil {
		return s.release(ctx, env, run)
	}
	db, _, err := build.EnsureForRunWithArtifact(ctx, s.cfg.Build, s.querier(env), env.Artifacts, run)
	if err != nil {
		return nil, nil, err
	}
	cfg, err := pgxpool.ParseConfig(db.URL)
	if err != nil {
		return nil, nil, errors.New("invalid release database URL")
	}
	cfg.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, nil, err
	}
	return pool, pool.Close, nil
}

func (*Stage) Rows(context.Context, stage.Env, artifact.Result) (func(context.Context, pgx.Tx, int64) error, error) {
	return nil, nil
}

func (s *Stage) Resolve(ctx context.Context, env stage.Env, run stage.Run, item stage.Item, art artifact.Result) (stage.Resolution, error) {
	if item.Key != itemKey(run) {
		return stage.Resolution{}, errors.New("evidence item identity mismatch")
	}
	file, _, err := readArtifactFindings(env.Artifacts.Abs(art.Path), art.SHA256)
	if err != nil {
		return stage.Resolution{}, errors.New(err.Error())
	}
	inputs, err := s.resolveInputs(ctx, env, run, false)
	if err != nil {
		return stage.Resolution{}, errors.New(err.Error())
	}
	if inputs.inputHash() != art.InputHash {
		return stage.Resolution{}, errors.New("evidence artifact does not match the run's frozen inputs")
	}
	if file.Bindings != inputs.bindings(run.Rel) {
		return stage.Resolution{}, errors.New("evidence bindings do not match the run")
	}
	return stage.Resolution{}, nil
}
