package run

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"go.temporal.io/api/serviceerror"
	workflowpb "go.temporal.io/api/workflow/v1"
)

// Report contains workshop progress and Temporal activity details.
type Report struct {
	AdoptedItems      int    `json:"adopted_items"`
	QualityPass       *bool  `json:"quality_pass,omitempty"`
	QualityReason     string `json:"quality_reason,omitempty"`
	ReleaseEligible   *bool  `json:"release_eligible,omitempty"`
	Run               store.Run
	Stages            map[string]stage.Counts
	PendingActivities []*workflowpb.PendingActivityInfo
}

// Status reports the recorded progress and pending Temporal activities.
func Status(ctx context.Context, d Deps, cc, rel string) (Report, error) {
	q := store.New(d.DB)
	row, err := q.GetRun(ctx, store.GetRunParams{Country: cc, Rel: rel})
	if err != nil {
		return Report{}, fmt.Errorf("get run: %w", err)
	}
	report := Report{Run: row, Stages: make(map[string]stage.Counts)}
	report.AdoptedItems, err = countAdoptedItems(ctx, q, row.ID, int(row.Generation))
	if err != nil {
		return Report{}, err
	}
	if row.State == "evaluated" {
		report.QualityReason = "evaluation artifact unavailable"
	}
	raw, err := q.GetBuildInventory(ctx, row.ID)
	if err != nil {
		return Report{}, fmt.Errorf("read evaluation inventory: %w", err)
	}
	var inventory struct {
		Items []struct {
			Stage, State string
			Path         *string
			Hash         *string `json:"artifact_sha256"`
		} `json:"items"`
	}
	if err = json.Unmarshal(raw, &inventory); err != nil {
		return Report{}, fmt.Errorf("decode evaluation inventory: %w", err)
	}
	for _, item := range inventory.Items {
		if item.Stage != "eval" || item.State != "done" {
			continue
		}
		report.loadQuality(d.Artifacts, item.Path, item.Hash)
	}
	counts, err := q.ListRunItemCounts(ctx, row.ID)
	if err != nil {
		return Report{}, fmt.Errorf("list item counts: %w", err)
	}
	for _, c := range counts {
		v := report.Stages[c.Stage]
		n := int(c.Count)
		v.Expected += n
		switch c.State {
		case "done":
			v.Done = n
		case "gap":
			v.Gap = n
		case "failed":
			v.Failed = n
		case "pending":
			v.Pending = n
		}
		report.Stages[c.Stage] = v
	}
	gaps, err := q.ListUnacceptedGapCounts(ctx, row.ID)
	if err != nil {
		return Report{}, fmt.Errorf("list gap counts: %w", err)
	}
	for _, g := range gaps {
		v := report.Stages[g.Stage]
		if v.UnacceptedGaps == nil {
			v.UnacceptedGaps = make(map[string]int)
		}
		v.UnacceptedGaps[g.Kind] = int(g.Count)
		report.Stages[g.Stage] = v
	}
	desc, err := d.Temporal.DescribeWorkflowExecution(ctx, WorkflowID(cc, rel), "")
	if err != nil {
		var missing *serviceerror.NotFound
		if !errors.As(err, &missing) {
			return Report{}, fmt.Errorf("describe workflow: %w", err)
		}
	} else {
		report.PendingActivities = desc.PendingActivities
	}
	return report, nil
}

func qualityFields(data []byte) (*bool, *bool, error) {
	var report struct {
		Status   string `json:"status"`
		Quality  *bool  `json:"quality_pass"`
		Eligible *bool  `json:"release_eligible"`
	}
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, nil, fmt.Errorf("decode evaluation report: %w", err)
	}
	if report.Status != "complete" || report.Quality == nil || report.Eligible == nil {
		return nil, nil, fmt.Errorf("invalid evaluation report")
	}
	return report.Quality, report.Eligible, nil
}

func artifactQuality(store artifact.Store, path, hash *string) (*bool, *bool, error) {
	if path == nil || hash == nil {
		return nil, nil, fmt.Errorf("missing evaluation artifact")
	}
	dir := store.Abs(*path)
	sum, err := artifact.Verify(dir)
	if err != nil || sum != *hash {
		return nil, nil, fmt.Errorf("invalid evaluation artifact")
	}
	data, err := os.ReadFile(filepath.Join(dir, "report.json"))
	if err != nil {
		return nil, nil, fmt.Errorf("read evaluation report: %w", err)
	}
	return qualityFields(data)
}

func (r *Report) loadQuality(store artifact.Store, path, hash *string) {
	var err error
	r.QualityPass, r.ReleaseEligible, err = artifactQuality(store, path, hash)
	if err != nil {
		r.QualityReason = "evaluation artifact unavailable"
	} else {
		r.QualityReason = ""
	}
}
