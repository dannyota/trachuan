package classify

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

type scannedLedger struct {
	*memoryLedger
	codec pgtype.TimestamptzCodec
}

func (m *scannedLedger) timestamp(value time.Time) (time.Time, error) {
	input := pgtype.Timestamptz{Time: value, Valid: true}
	encoded, err := m.codec.PlanEncode(nil, pgtype.TimestamptzOID, pgtype.BinaryFormatCode, input).Encode(input, nil)
	if err != nil {
		return time.Time{}, fmt.Errorf("encode synthetic PostgreSQL timestamp: %w", err)
	}
	var output pgtype.Timestamptz
	if err = m.codec.PlanScan(nil, pgtype.TimestamptzOID, pgtype.BinaryFormatCode, &output).Scan(encoded, &output); err != nil {
		return time.Time{}, fmt.Errorf("scan synthetic PostgreSQL timestamp: %w", err)
	}
	return output.Time, nil
}

func (m *scannedLedger) List(ctx context.Context) ([]Submission, error) {
	rows, err := m.memoryLedger.List(ctx)
	if err != nil {
		return nil, err
	}
	for i := range rows {
		if rows[i].CreatedAt, err = m.timestamp(rows[i].CreatedAt); err != nil {
			return nil, err
		}
		for _, target := range []**time.Time{&rows[i].SubmittedAt, &rows[i].DeadlineAt} {
			if *target == nil {
				continue
			}
			value, scanErr := m.timestamp(**target)
			if scanErr != nil {
				return nil, scanErr
			}
			*target = &value
		}
	}
	return rows, nil
}

func TestCancel_PreservesReceiptAfterPostgresTimezoneScan(t *testing.T) {
	for _, offset := range []int{7 * 3600, -5 * 3600} {
		t.Run(fmt.Sprint(offset), func(t *testing.T) {
			r, ledger, client, o := screenFixture(t)
			r.Repository = &scannedLedger{memoryLedger: ledger, codec: pgtype.TimestamptzCodec{ScanLocation: time.FixedZone("synthetic", offset)}}
			client.ended = false
			if _, err := r.Screen(context.Background(), o, documents(), families()); err == nil {
				t.Fatal("expected interrupted polling")
			}
			path := filepath.Join(r.IntentDir, "submissions", ledger.rows[0].ID+".batch")
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			client.ended = true
			if err = r.Cancel(context.Background(), ledger.rows[0].ID, o.Out); err != nil {
				t.Fatal("cancellation could not reuse immutable receipt", err)
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) {
				t.Fatal("cancellation changed the submission receipt")
			}
			if ledger.rows[0].State != "cancelled" || !ledger.rows[0].Settled || ledger.rows[0].CostMicroUSD != 8 {
				t.Fatalf("cancelled ledger row %+v", ledger.rows[0])
			}
		})
	}
}

func TestBatchReceipt_ConcurrentTimezoneWritersPreserveBytes(t *testing.T) {
	r, ledger, client, o := screenFixture(t)
	client.ended = false
	if _, err := r.Screen(context.Background(), o, documents(), families()); err == nil {
		t.Fatal("expected interrupted polling")
	}
	reader := &scannedLedger{memoryLedger: ledger, codec: pgtype.TimestamptzCodec{ScanLocation: time.FixedZone("synthetic", 7*3600)}}
	rows, err := reader.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(r.IntentDir, "submissions", ledger.rows[0].ID+".batch")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	done := make(chan error, 2)
	for _, s := range []Submission{ledger.rows[0], rows[0]} {
		go func(s Submission) { <-start; done <- r.writeBatch(s) }(s)
	}
	close(start)
	for range 2 {
		if err = <-done; err != nil {
			t.Error("concurrent writer disagreed with immutable receipt", err)
		}
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("concurrent writers changed the receipt")
	}
}
