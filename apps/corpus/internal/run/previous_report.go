package run

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/evaluation"
	"danny.vn/trachuan/apps/corpus/internal/stage"
)

const previousReportStage = "previous-report"

type ReportChoice struct {
	Path string
	None bool
}

func (c ReportChoice) given() bool { return c.Path != "" || c.None }

type decisionInput struct {
	Choice       ReportChoice
	Existing     bool
	Country, Rel string
	Snapshot     *stage.Snapshot
	Artifacts    artifact.Store
	HasEvalItem  func(context.Context) (bool, error)
}

type chosenReport struct {
	sha256 string
	data   []byte
}

func decide(ctx context.Context, in decisionInput) (string, error) {
	if in.Choice.Path != "" && in.Choice.None {
		return "", errors.New("use only one of --previous-report and --no-previous-report")
	}
	current := in.Snapshot.PreviousReport
	older := in.Existing && stage.PredatesDecision(in.Country, in.Rel, *in.Snapshot)
	if !in.Choice.given() {
		if !in.Existing {
			return "", errors.New("a new run requires --previous-report <file> or --no-previous-report")
		}
		if current == nil && !older {
			return "", errors.New("run has no previous report decision; start it with --previous-report <file> or --no-previous-report")
		}
		return "", nil
	}
	if older {
		return "", errors.New("run predates the previous report decision; start it without --previous-report or --no-previous-report")
	}
	chosen, err := loadChoice(in.Choice, in.Country, in.Rel)
	if err != nil {
		return "", err
	}
	if current != nil && current.SHA256 == chosen.sha256 {
		return "", nil
	}
	if in.Existing {
		evaluated, lookupErr := in.HasEvalItem(ctx)
		if lookupErr != nil {
			return "", fmt.Errorf("check eval item: %w", lookupErr)
		}
		if evaluated {
			return "", errors.New("run has an eval item; its previous report decision cannot change")
		}
	}
	decision, err := freezeChoice(ctx, in.Artifacts, chosen)
	if err != nil {
		return "", err
	}
	in.Snapshot.PreviousReport = &decision
	switch {
	case !in.Existing:
		return "", nil
	case current == nil:
		return "previous report decision set: " + decision.SHA256, nil
	}
	return fmt.Sprintf("previous report decision replaced: %s -> %s", current.SHA256, decision.SHA256), nil
}

func loadChoice(choice ReportChoice, country, rel string) (chosenReport, error) {
	if choice.None {
		return chosenReport{sha256: stage.NoPreviousReport}, nil
	}
	data, err := os.ReadFile(choice.Path)
	if err != nil {
		return chosenReport{}, fmt.Errorf("read previous report: %w", err)
	}
	report, err := evaluation.ParseDecisionReport(data)
	if err != nil {
		return chosenReport{}, err
	}
	if report.Country != country {
		return chosenReport{}, fmt.Errorf("previous report country %q is not %q", report.Country, country)
	}
	if err = validateRel(report.Rel); err != nil {
		return chosenReport{}, fmt.Errorf("previous report: %w", err)
	}
	if stage.CompareRel(report.Rel, rel) >= 0 {
		return chosenReport{}, fmt.Errorf("previous report release %s is not below %s", report.Rel, rel)
	}
	sum := sha256.Sum256(data)
	return chosenReport{sha256: hex.EncodeToString(sum[:]), data: data}, nil
}

func freezeChoice(ctx context.Context, artifacts artifact.Store, chosen chosenReport) (stage.PreviousReport, error) {
	if chosen.data == nil {
		return stage.PreviousReport{SHA256: chosen.sha256}, nil
	}
	result, err := artifacts.PublishContent(ctx, previousReportStage, stage.RecipeHash(previousReportStage, "1"), func(dir string) error {
		return os.WriteFile(filepath.Join(dir, stage.PreviousReportFile), chosen.data, 0600)
	})
	if err != nil {
		return stage.PreviousReport{}, fmt.Errorf("store previous report: %w", err)
	}
	return stage.PreviousReport{SHA256: chosen.sha256, Path: result.Path}, nil
}
