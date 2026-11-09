package run

import (
	"context"
	"errors"
	"fmt"

	"danny.vn/traquydinh/apps/corpus/internal/stage"
	"danny.vn/traquydinh/apps/corpus/internal/store"
	"go.temporal.io/api/serviceerror"
	workflowpb "go.temporal.io/api/workflow/v1"
)

// Report contains workshop progress and Temporal activity details.
type Report struct {
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
