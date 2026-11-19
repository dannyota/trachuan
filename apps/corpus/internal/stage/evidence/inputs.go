package evidence

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/detector"
	"danny.vn/trachuan/apps/corpus/internal/evaluation"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"github.com/jackc/pgx/v5"
)

type inputError struct{ reason string }

func (e *inputError) Error() string { return e.reason }

func refuse(format string, args ...any) error {
	return &inputError{reason: fmt.Sprintf(format, args...)}
}

type inventoryItem struct {
	Stage          string  `json:"stage"`
	ItemKey        string  `json:"item_key"`
	State          string  `json:"state"`
	Path           *string `json:"path"`
	ArtifactSHA256 *string `json:"artifact_sha256"`
}

type gapRow struct {
	Stage   string `json:"stage"`
	ItemKey string `json:"item_key"`
	Kind    string `json:"kind"`
	Detail  string `json:"detail"`
}

type inventory struct {
	Items []inventoryItem `json:"items"`
	Gaps  []gapRow        `json:"gaps"`
}

func readInventory(ctx context.Context, q store.Querier, runID int64) (inventory, error) {
	raw, err := q.GetBuildInventory(ctx, runID)
	if err != nil {
		return inventory{}, fmt.Errorf("read run items: %w", err)
	}
	var inv inventory
	if err = json.Unmarshal(raw, &inv); err != nil {
		return inventory{}, fmt.Errorf("decode run items: %v", err)
	}
	return inv, nil
}

func (inv inventory) doneArtifact(stageName, key string) (path, sum string, ok bool) {
	for _, item := range inv.Items {
		if item.Stage == stageName && item.ItemKey == key && item.State == "done" && item.Path != nil && item.ArtifactSHA256 != nil {
			return *item.Path, *item.ArtifactSHA256, true
		}
	}
	return "", "", false
}

func (inv inventory) gapSnapshot() (string, error) {
	rows := make([]gapRow, 0, len(inv.Gaps))
	for _, gap := range inv.Gaps {
		if gap.Stage != stage.EvidenceStage {
			rows = append(rows, gap)
		}
	}
	slices.SortFunc(rows, func(a, b gapRow) int {
		return strings.Compare(a.Stage+"\x00"+a.ItemKey+"\x00"+a.Kind+"\x00"+a.Detail, b.Stage+"\x00"+b.ItemKey+"\x00"+b.Kind+"\x00"+b.Detail)
	})
	return hashValue(rows)
}

type predecessor struct {
	rel, findingsSHA256 string
	file                *File
	identitySHA256      string
	identityPath        string
}

type frozenInputs struct {
	build        artifact.Result
	buildDir     string
	reportPath   string
	reportSHA256 string
	dumpSHA256   string
	gapSnapshot  string
	knowledge    []byte
	predecessor  predecessor
}

func bareSHA256(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func (f frozenInputs) knowledgeHash() string {
	if f.knowledge == nil {
		return noneValue
	}
	return bareSHA256(f.knowledge)
}

func (f frozenInputs) inputHash() string {
	return stage.RecipeHash("evidence-input", "1", f.build.SHA256, f.reportSHA256, f.gapSnapshot, f.knowledgeHash(), f.predecessor.rel, f.predecessor.findingsSHA256, f.predecessor.identitySHA256)
}

func prefixedOrNone(sum string) string {
	if sum == noneValue {
		return noneValue
	}
	return hashPrefix + sum
}

func (f frozenInputs) identityBindings(rel string) identityBindings {
	return identityBindings{Rel: rel, DumpSHA256: hashPrefix + f.dumpSHA256, BuildArtifactSHA256: hashPrefix + f.build.SHA256, ReportSHA256: hashPrefix + f.reportSHA256}
}

func (f frozenInputs) changesBindings(rel string) changesBindings {
	return changesBindings{identityBindings: f.identityBindings(rel), PredecessorRel: f.predecessor.rel, PredecessorIdentitySHA256: prefixedOrNone(f.predecessor.identitySHA256)}
}

func (f frozenInputs) bindings(rel string) Bindings {
	return Bindings{Rel: rel, DumpSHA256: hashPrefix + f.dumpSHA256, BuildArtifactSHA256: hashPrefix + f.build.SHA256, ReportSHA256: hashPrefix + f.reportSHA256, DetectorRegistrySHA256: hashPrefix + detector.Hash(), PredecessorRel: f.predecessor.rel, PredecessorFindingsSHA256: prefixedOrNone(f.predecessor.findingsSHA256)}
}

func (s *Stage) committedBuild(ctx context.Context, env stage.Env, q store.Querier, run stage.Run, inv inventory, verifyDump bool) (artifact.Result, error) {
	path, sum, ok := inv.doneArtifact("build", itemKey(run))
	if !ok {
		return artifact.Result{}, refuse("run has no done build item")
	}
	if verifyDump {
		verified, err := s.lookupBuild(ctx, env, q, run)
		if err != nil {
			return artifact.Result{}, fmt.Errorf("verify build: %w", err)
		}
		if verified.SHA256 != sum {
			return artifact.Result{}, errors.New("verified build differs from the run's build item")
		}
		return verified, nil
	}
	manifest, err := artifact.VerifyFiles(env.Artifacts.Abs(path), "build.json")
	if err != nil || manifest != sum {
		return artifact.Result{}, refuse("build artifact failed verification")
	}
	return artifact.Result{Path: path, SHA256: sum}, nil
}

func (s *Stage) resolveInputs(ctx context.Context, env stage.Env, run stage.Run, verifyDump bool) (frozenInputs, error) {
	decision := run.Snapshot.PreviousReport
	if decision == nil {
		return frozenInputs{}, refuse("run has no previous report decision")
	}
	q := s.querier(env)
	inv, err := readInventory(ctx, q, run.ID)
	if err != nil {
		return frozenInputs{}, err
	}
	build, err := s.committedBuild(ctx, env, q, run, inv, verifyDump)
	if err != nil {
		return frozenInputs{}, err
	}
	key := itemKey(run)
	path, sum, ok := inv.doneArtifact("eval", key)
	if !ok {
		return frozenInputs{}, refuse("run has no done eval item")
	}
	dir := env.Artifacts.Abs(path)
	manifest, err := artifact.VerifyFiles(dir, "report.json")
	if err != nil || manifest != sum {
		return frozenInputs{}, refuse("selected report artifact failed verification")
	}
	reportPath := filepath.Join(dir, "report.json")
	data, err := os.ReadFile(reportPath)
	if err != nil {
		return frozenInputs{}, fmt.Errorf("read selected report: %w", err)
	}
	var report struct {
		Status              string `json:"status"`
		Country             string `json:"country"`
		Rel                 string `json:"rel"`
		BuildArtifactSHA256 string `json:"build_artifact_sha256"`
		DumpSHA256          string `json:"dump_sha256"`
		PreviousDiff        any    `json:"previous_diff"`
	}
	if err = json.Unmarshal(data, &report); err != nil {
		return frozenInputs{}, refuse("decode selected report: %v", err)
	}
	if report.Status != "complete" || report.Country != run.Country || report.Rel != run.Rel {
		return frozenInputs{}, refuse("selected report does not belong to %s", key)
	}
	if report.BuildArtifactSHA256 != build.SHA256 {
		return frozenInputs{}, refuse("selected report does not bind the run's build")
	}
	if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(report.DumpSHA256) {
		return frozenInputs{}, refuse("selected report has no dump SHA-256")
	}
	compared, err := evaluation.Report{PreviousDiff: report.PreviousDiff}.ComparedRelease()
	if err != nil {
		return frozenInputs{}, refuse("selected report: %v", err)
	}
	gaps, err := inv.gapSnapshot()
	if err != nil {
		return frozenInputs{}, err
	}
	buildDir := env.Artifacts.Abs(build.Path)
	knowledge, err := readKnowledge(buildDir, build.SHA256)
	if err != nil {
		return frozenInputs{}, err
	}
	previous, err := resolvePredecessor(ctx, env, q, run, *decision, compared)
	if err != nil {
		return frozenInputs{}, err
	}
	return frozenInputs{build: build, buildDir: buildDir, reportPath: reportPath, reportSHA256: bareSHA256(data), dumpSHA256: report.DumpSHA256, gapSnapshot: gaps, knowledge: knowledge, predecessor: previous}, nil
}

func resolvePredecessor(ctx context.Context, env stage.Env, q store.Querier, run stage.Run, decision stage.PreviousReport, compared *evaluation.ComparedRelease) (predecessor, error) {
	if decision.None() {
		if compared != nil {
			return predecessor{}, refuse("selected report names %s, previous report decision is none", compared.Rel)
		}
		return predecessor{rel: noneValue, findingsSHA256: noneValue, identitySHA256: noneValue}, nil
	}
	if compared == nil || compared.ReportSHA256 != decision.SHA256 || compared.Country != run.Country {
		return predecessor{}, refuse("selected report does not match the previous report decision %s", decision.SHA256)
	}
	row, err := q.GetRun(ctx, store.GetRunParams{Country: run.Country, Rel: compared.Rel})
	if errors.Is(err, pgx.ErrNoRows) {
		return predecessor{}, refuse("predecessor %s/%s has no run", run.Country, compared.Rel)
	}
	if err != nil {
		return predecessor{}, fmt.Errorf("read predecessor run: %w", err)
	}
	var snapshot stage.Snapshot
	if err = json.Unmarshal(row.Snapshot, &snapshot); err != nil {
		return predecessor{}, refuse("decode predecessor snapshot: %v", err)
	}
	if stage.PredatesDecision(run.Country, compared.Rel, snapshot) {
		return predecessor{rel: compared.Rel, findingsSHA256: noneValue, identitySHA256: noneValue}, nil
	}
	if snapshot.PreviousReport == nil {
		return predecessor{}, refuse("predecessor %s has no previous report decision", compared.Rel)
	}
	if row.State != "evaluated" && row.State != "released" {
		return predecessor{}, refuse("predecessor %s is %s, not evaluated", compared.Rel, row.State)
	}
	file, sum, dir, err := readRunEvidence(ctx, q, env.Artifacts, row)
	if err != nil {
		return predecessor{}, err
	}
	bound, hasPrefix := strings.CutPrefix(file.Bindings.ReportSHA256, hashPrefix)
	if !hasPrefix || bound != decision.SHA256 || file.Bindings.Rel != compared.Rel {
		return predecessor{}, refuse("predecessor %s evidence does not bind the frozen previous report", compared.Rel)
	}
	path, identity, err := predecessorIdentity(dir, compared.Rel, decision.SHA256)
	if err != nil {
		return predecessor{}, err
	}
	return predecessor{rel: compared.Rel, findingsSHA256: sum, file: &file, identitySHA256: identity, identityPath: path}, nil
}

func readRunEvidence(ctx context.Context, q store.Querier, artifacts artifact.Store, row store.Run) (File, string, string, error) {
	inv, err := readInventory(ctx, q, row.ID)
	if err != nil {
		return File{}, "", "", err
	}
	path, sum, ok := inv.doneArtifact(stage.EvidenceStage, row.Country+"/"+row.Rel)
	if !ok {
		return File{}, "", "", refuse("%s/%s has no done evidence item", row.Country, row.Rel)
	}
	dir := artifacts.Abs(path)
	file, findings, err := readArtifactFindings(dir, sum)
	return file, findings, dir, err
}

func readArtifactFindings(dir, manifestSHA256 string) (File, string, error) {
	verified, err := artifact.Verify(dir)
	if err != nil || verified != manifestSHA256 {
		return File{}, "", refuse("evidence artifact failed verification")
	}
	data, err := os.ReadFile(filepath.Join(dir, findingsFile))
	if err != nil {
		return File{}, "", refuse("read findings: %v", err)
	}
	var file File
	if err = json.Unmarshal(data, &file); err != nil {
		return File{}, "", refuse("decode findings: %v", err)
	}
	if file.Schema != schemaVersion {
		return File{}, "", refuse("findings schema %d is not %d", file.Schema, schemaVersion)
	}
	return file, bareSHA256(data), nil
}

func LoadFindings(ctx context.Context, q store.Querier, artifacts artifact.Store, country, rel string) (File, error) {
	row, err := q.GetRun(ctx, store.GetRunParams{Country: country, Rel: rel})
	if err != nil {
		return File{}, fmt.Errorf("read run %s/%s: %w", country, rel, err)
	}
	file, _, _, err := readRunEvidence(ctx, q, artifacts, row)
	return file, err
}
