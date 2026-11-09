package run

import (
	"context"
	"fmt"
	"os"

	enumspb "go.temporal.io/api/enums/v1"
	historypb "go.temporal.io/api/history/v1"
	"google.golang.org/protobuf/encoding/protojson"
)

// ExportHistory writes one workflow execution history in Temporal JSON format.
func ExportHistory(ctx context.Context, d Deps, workflowID, runID, path string) error {
	iter := d.Temporal.GetWorkflowHistory(ctx, workflowID, runID, false, enumspb.HISTORY_EVENT_FILTER_TYPE_ALL_EVENT)
	history := &historypb.History{}
	for iter.HasNext() {
		event, err := iter.Next()
		if err != nil {
			return fmt.Errorf("read history: %w", err)
		}
		history.Events = append(history.Events, event)
	}
	data, err := protojson.MarshalOptions{Indent: "  "}.Marshal(history)
	if err != nil {
		return fmt.Errorf("encode history: %w", err)
	}
	if err = os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write history: %w", err)
	}
	return nil
}
