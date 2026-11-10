//go:build integration

package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/run"
	"danny.vn/trachuan/apps/corpus/internal/sourcegate"
	"danny.vn/trachuan/apps/corpus/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/mock"
	"go.temporal.io/api/serviceerror"
	workflowservice "go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/mocks"
)

func resumeFixture(t *testing.T) (run.Deps, int64) {
	t.Helper()
	ctx := context.Background()
	db, err := pgxpool.New(ctx, testdb.New(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	var id int64
	if err = db.QueryRow(ctx, `INSERT INTO run (country,rel,state,snapshot) VALUES ('vn','20990101','failed','{}') RETURNING id`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	temporal := &mocks.Client{}
	temporal.On("DescribeWorkflowExecution", mock.Anything, run.WorkflowID("vn", "20990101"), "").Return((*workflowservice.DescribeWorkflowExecutionResponse)(nil), serviceerror.NewNotFound("missing"))
	return run.Deps{DB: db, Temporal: temporal}, id
}

func TestSourceResume_PausedSourceStaysPausedAfterRestart(t *testing.T) {
	ctx := context.Background()
	d, id := resumeFixture(t)
	if err := sourcegate.New(d.DB).Pause(ctx, id, "chinhphu", "challenge", ""); err != nil {
		t.Fatal(err)
	}
	if err := sourcegate.New(d.DB).Admit(ctx, id, "chinhphu"); !errors.Is(err, sourcegate.ErrSourcePaused) {
		t.Fatalf("admit: %v", err)
	}
}

func TestSourceResume_ClearsPauseExplicitly(t *testing.T) {
	ctx := context.Background()
	d, id := resumeFixture(t)
	if err := sourcegate.New(d.DB).Pause(ctx, id, "chinhphu", "challenge", ""); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := resumeSource(ctx, d, "vn", "20990101", "chinhphu", "reviewed", &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "challenge") {
		t.Fatalf("output: %s", out.String())
	}
	if err := sourcegate.New(d.DB).Admit(ctx, id, "chinhphu"); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := d.DB.QueryRow(ctx, `SELECT count(*) FROM run_source_event WHERE run_id=$1 AND event='clear' AND clear_note='reviewed'`, id).Scan(&count); err != nil || count != 1 {
		t.Fatalf("clear events=%d err=%v", count, err)
	}
}

func TestSourceResume_RejectsReleasedRun(t *testing.T) {
	ctx := context.Background()
	d, id := resumeFixture(t)
	if err := sourcegate.New(d.DB).Pause(ctx, id, "vbpl", "challenge", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := d.DB.Exec(ctx, `UPDATE run SET state='released' WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if err := resumeSource(ctx, d, "vn", "20990101", "vbpl", "reviewed", &bytes.Buffer{}); err == nil {
		t.Fatal("released run accepted")
	}
	if err := sourcegate.New(d.DB).Admit(ctx, id, "vbpl"); !errors.Is(err, sourcegate.ErrSourcePaused) {
		t.Fatalf("pause lost: %v", err)
	}
}
