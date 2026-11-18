package evidence

import (
	"context"
	"fmt"

	"danny.vn/trachuan/apps/corpus/internal/detector"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Subject struct {
	Type   string   `json:"type"`
	Fields []string `json:"fields"`
}

func (s Subject) validate() error {
	want := map[string]int{"document": 1, "provision": 2, "relation": 3, "claim": 3, "check_item": 4, "report": 2, "absent_document": 1}[s.Type]
	if want == 0 {
		return fmt.Errorf("unknown subject type %q", s.Type)
	}
	if len(s.Fields) != want {
		return fmt.Errorf("subject %s has %d fields, want %d", s.Type, len(s.Fields), want)
	}
	return nil
}

type Fired struct {
	Subject           Subject
	Discriminator     string
	Severity          detector.Severity
	FingerprintInputs []string
}

type Observation struct {
	Findings           []Fired
	Observed           int
	NotObservable      int
	Excluded           int
	ObservedCompletely func(Subject) bool
}

type Inputs struct {
	Run        stage.Run
	BuildDir   string
	ReportPath string
	Release    *pgxpool.Pool

	ArtifactDir func(stage, itemKey string) (string, bool, error)
}

type Detector interface {
	Spec() detector.Detector
	Detect(context.Context, Inputs) (Observation, error)
}

type IncompleteError struct {
	Detector string
	Reason   string
}

func (e *IncompleteError) Error() string {
	if e.Detector == "" {
		return "detector_incomplete: " + e.Reason
	}
	return "detector_incomplete: " + e.Detector + ": " + e.Reason
}
