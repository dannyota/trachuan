// Package extract converts VBPL trees into provision artifacts.
package extract

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/ocrstore"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/membership"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"danny.vn/trachuan/apps/corpus/internal/vbpl"
	"github.com/jackc/pgx/v5"
)

// ReleaseChecker identifies the completed OCR checks used for releases.
const ReleaseChecker = "claude-opus-5-5/medium/ocr-check-2"

// Stage extracts provisions from fetch artifacts.
type Stage struct {
	ocr     ocrReader
	engines []string
	checker string
}

// New constructs an extract stage.
func New() *Stage { return &Stage{engines: AcceptedEngines(), checker: ReleaseChecker} }

// AcceptedEngines returns the recipe's engine preference in a new slice.
func AcceptedEngines() []string {
	return []string{"vision-ocr/builtin-latest", "vision-ocr/builtin-latest/poppler-200", "vision-ocr/builtin-latest/poppler-200-jpeg"}
}

// WithOCR configures the store and ordered engines. An empty checker keeps the
// current completed-check identity.
func (s *Stage) WithOCR(store *ocrstore.Store, engines []string, checker string) *Stage {
	if store != nil {
		s.ocr = store
	} else {
		s.ocr = nil
	}
	if engines != nil {
		s.engines = append([]string{}, engines...)
	}
	if checker != "" {
		s.checker = checker
	}
	return s
}

// Name returns the stage name.
func (*Stage) Name() string { return "extract" }

// Recipe returns the extract recipe hash.
func (s *Stage) Recipe(stage.Snapshot) string {
	parts := append([]string{"html-transcription/1", "textparse/7", "heading-only/1", "ocr-store/2", s.checker}, s.engines...)
	parts = append(parts, manualMismatchRecipe()...)
	return stage.RecipeHash("extract", "tree/1", parts...)
}

// Plan selects completed fetch artifacts for this run.
func (s *Stage) Plan(ctx context.Context, env stage.Env, run stage.Run) ([]stage.Item, error) {
	q := store.New(env.DB)
	active, err := membership.FinalActiveKeys(ctx, q, run)
	if err != nil {
		return nil, err
	}
	inputs, err := q.ListExtractInputs(ctx, run.ID)
	if err != nil {
		return nil, fmt.Errorf("list fetch artifacts: %w", err)
	}
	items := make([]stage.Item, 0, len(inputs))
	for _, input := range inputs {
		if active != nil && !active[input.ItemKey] {
			continue
		}
		entry, getErr := q.GetExtractInput(ctx, store.GetExtractInputParams{RunID: run.ID, ItemKey: input.ItemKey})
		if getErr != nil {
			return nil, fmt.Errorf("load fetch artifact: %w", getErr)
		}
		selected, selectErr := planFrozenSelection(ctx, q, run.ID, input.ItemKey, func() (selection, error) {
			choice, _, err := chooseWithOCR(ctx, env.Artifacts.Abs(entry.Path), input.ItemKey, s.ocr, s.engines, s.checker)
			if errors.Is(err, vbpl.ErrTreeTooLarge) {
				return selection{source: "source_tree"}, nil
			}
			return choice, err
		})
		if selectErr != nil {
			return nil, fmt.Errorf("select text source: %w", selectErr)
		}
		if selected.consulted && selected.gap != "text_number_mismatch" {
			mismatch, checkErr := frozenMismatch(env.Artifacts.Abs(entry.Path), input.ItemKey)
			if checkErr != nil {
				return nil, fmt.Errorf("check frozen selection: %w", checkErr)
			}
			if mismatch.gap != "" {
				selected = mismatch
			}
		}
		items = append(items, stage.Item{Key: input.ItemKey, InputHash: selectionInputHash(input.Sha256, selected)})
	}
	return items, nil
}

// Process publishes the extracted provisions and gaps.
func (s *Stage) Process(ctx context.Context, env stage.Env, run stage.Run, item stage.Item) (stage.Output, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	input, err := store.New(env.DB).GetExtractInput(ctx, store.GetExtractInputParams{RunID: run.ID, ItemKey: item.Key})
	if err != nil {
		return stage.Output{}, fmt.Errorf("load fetch artifact: %w", err)
	}
	if verifyErr := verifyFetch(env.Artifacts.Abs(input.Path), input.Sha256); verifyErr != nil {
		return stage.Output{}, verifyErr
	}
	q := store.New(env.DB)
	row, lookupErr := q.GetRunOCRSelection(ctx, store.GetRunOCRSelectionParams{RunID: run.ID, ItemKey: item.Key})
	var selected selection
	var nodes []vbpl.Node
	if lookupErr == nil {
		selected, err = selectionFromRow(row)
		if err == nil && selected.gap != "text_number_mismatch" {
			var mismatch selection
			mismatch, err = frozenMismatch(env.Artifacts.Abs(input.Path), item.Key)
			if mismatch.gap != "" {
				selected = mismatch
			}
		}
		if err == nil {
			selected, nodes, err = materializeFrozen(ctx, env.Artifacts.Abs(input.Path), item.Key, selected)
		}
	} else if errors.Is(lookupErr, pgx.ErrNoRows) {
		selected, nodes, err = choose(ctx, env.Artifacts.Abs(input.Path), item.Key)
		if err == nil && selected.consulted {
			return stage.Output{}, fmt.Errorf("missing frozen OCR selection")
		}
	} else {
		return stage.Output{}, fmt.Errorf("read OCR selection: %w", lookupErr)
	}
	if errors.Is(err, vbpl.ErrTreeTooLarge) {
		return stage.Output{State: "gap", Gaps: []stage.Gap{{Kind: "tree_too_large"}}}, nil
	}
	if err != nil {
		return stage.Output{}, fmt.Errorf("select text source: %w", err)
	}
	if selectionInputHash(input.Sha256, selected) != item.InputHash {
		return stage.Output{}, stage.Retryable(fmt.Errorf("text selection changed for %q", item.Key))
	}
	var rows []provision
	var gaps []stage.Gap
	var record *textRecord
	switch selected.source {
	case "source_tree":
		rows, gaps, err = extractTree(ctx, nodes)
		if err != nil {
			return stage.Output{}, fmt.Errorf("extract tree: %w", err)
		}
		if selected.gap != "" {
			gaps = append(gaps, stage.Gap{Kind: selected.gap, Detail: selected.reason})
		}
	case "html_transcription":
		if selected.tooLarge {
			rows = []provision{}
			gaps = []stage.Gap{{Kind: "text_limit", Detail: "file exceeds 50000000 bytes"}, {Kind: "no_tree"}}
			record = &textRecord{TextSource: selected.source, FileName: selected.name, SHA256: selected.hash}
		} else {
			rows, gaps, record = parseTranscription(selected.data, selected.name, selected.hash)
		}
	case "ocr":
		if s.ocr == nil {
			return stage.Output{}, fmt.Errorf("OCR store is not configured")
		}
		data, readErr := s.ocr.ReadText(selected.entry.Text)
		if readErr != nil {
			return stage.Output{}, fmt.Errorf("read OCR text: %w", readErr)
		}
		rows, gaps, record = parseOCR(selected, data)
	default:
		rows = []provision{}
		gaps = []stage.Gap{{Kind: "no_tree"}, {Kind: selected.gap, Detail: selected.reason}}
		record = &textRecord{TextSource: "none", Gap: selected.gap}
	}

	if record != nil {
		record.Tree = selected.tree
	}

	art, err := env.Artifacts.Publish(ctx, s.Name(), item.InputHash, s.Recipe(run.Snapshot), func(dir string) error {
		for _, file := range []struct {
			name  string
			value any
		}{{"provisions.json", rows}, {"report.json", gaps}} {
			b, marshalErr := json.Marshal(file.value)
			if marshalErr != nil {
				return fmt.Errorf("encode %s: %w", file.name, marshalErr)
			}
			if writeErr := os.WriteFile(filepath.Join(dir, file.name), b, 0600); writeErr != nil {
				return fmt.Errorf("write %s: %w", file.name, writeErr)
			}
		}
		if record != nil {
			b, marshalErr := json.Marshal(record)
			if marshalErr != nil {
				return fmt.Errorf("encode text.json: %w", marshalErr)
			}
			if writeErr := os.WriteFile(filepath.Join(dir, "text.json"), b, 0600); writeErr != nil {
				return fmt.Errorf("write text.json: %w", writeErr)
			}
		}
		return ctx.Err()
	})
	if err != nil {
		return stage.Output{}, fmt.Errorf("publish extract artifact: %w", err)
	}
	return stage.Output{Artifact: &art, State: "done"}, nil
}

// Rows loads provision rows from the artifact.
func (*Stage) Rows(_ context.Context, env stage.Env, art artifact.Result) (func(context.Context, pgx.Tx, int64) error, error) {
	data, err := os.ReadFile(filepath.Join(env.Artifacts.Abs(art.Path), "provisions.json"))
	if err != nil {
		return nil, fmt.Errorf("read provisions: %w", err)
	}
	var rows []provision
	if err = json.Unmarshal(data, &rows); err != nil {
		return nil, fmt.Errorf("decode provisions: %w", err)
	}
	return func(ctx context.Context, tx pgx.Tx, id int64) error {
		q := store.New(tx)
		for _, row := range rows {
			if insertErr := q.InsertProvision(ctx, store.InsertProvisionParams{ArtifactID: id, NodeID: row.NodeID, ParentNodeID: row.ParentNodeID, Kind: row.Kind, Label: row.Label, Heading: row.Heading, Ordinal: int32(row.Ordinal), Path: row.Path, Text: row.Text}); insertErr != nil {
				return fmt.Errorf("insert provision: %w", insertErr)
			}
		}
		return nil
	}, nil
}

// Resolve returns gaps recorded in the artifact.
func (*Stage) Resolve(_ context.Context, env stage.Env, _ stage.Run, _ stage.Item, art artifact.Result) (stage.Resolution, error) {
	data, err := os.ReadFile(filepath.Join(env.Artifacts.Abs(art.Path), "report.json"))
	if err != nil {
		return stage.Resolution{}, fmt.Errorf("read extract report: %w", err)
	}
	var gaps []stage.Gap
	if err = json.Unmarshal(data, &gaps); err != nil {
		return stage.Resolution{}, fmt.Errorf("decode extract report: %w", err)
	}
	return stage.Resolution{Gaps: gaps}, nil
}

func verifyFetch(path, expected string) error {
	sum, err := artifact.Verify(path)
	if err != nil {
		return stage.Retryable(fmt.Errorf("verify fetch artifact: %w", err))
	}
	if sum != expected {
		return stage.Retryable(fmt.Errorf("fetch artifact hash mismatch: got %s, want %s", sum, expected))
	}
	return nil
}
