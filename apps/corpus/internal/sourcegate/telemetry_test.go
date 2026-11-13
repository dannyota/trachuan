package sourcegate

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestTelemetry_OutcomeAndColumns(t *testing.T) {
	for _, tc := range []struct {
		name    string
		status  int
		err     error
		outcome string
	}{
		{"ok", 200, nil, "ok"}, {"rate limit", 429, nil, "http_error"}, {"invalid body", 200, ErrInvalidBody, "invalid_body"}, {"timeout", 0, context.DeadlineExceeded, "timeout"}, {"transport", 0, errors.New("failure"), "transport"}, {"cancelled", 0, context.Canceled, "cancelled"}, {"paused", 403, ErrSourcePaused, "paused"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, collector := WithTelemetry(context.Background())
			RecordAttempt(WithAttempt(ctx, 2), "vbpl", "detail", time.Now(), tc.status, tc.err)
			rows := collector.Records()
			if len(rows) != 1 || rows[0].Outcome != tc.outcome || rows[0].Attempt != 2 || rows[0].StartedAt.IsZero() || rows[0].DurationMS < 0 {
				t.Fatalf("rows=%+v", rows)
			}
			if tc.status == 0 && rows[0].Status != nil || tc.status != 0 && (rows[0].Status == nil || *rows[0].Status != tc.status) {
				t.Fatalf("status=%v", rows[0].Status)
			}
		})
	}
	typ := reflect.TypeOf(RequestRecord{})
	want := []string{"Source", "Endpoint", "Outcome", "Attempt", "StartedAt", "DurationMS", "Status"}
	if typ.NumField() != len(want) {
		t.Fatalf("fields=%v", typ)
	}
	for i, name := range want {
		if typ.Field(i).Name != name {
			t.Fatalf("field=%s", typ.Field(i).Name)
		}
	}
}
