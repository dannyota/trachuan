//go:build integration

package classify

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/batchapi"
	"github.com/anthropics/anthropic-sdk-go/v2"
)

type pollingClient struct {
	mu                       sync.Mutex
	requests                 []batchapi.Request
	ended                    bool
	cancellations, downloads int
	message                  anthropic.Message
}

func (p *pollingClient) CountTokens(context.Context, anthropic.MessageNewParams) (int64, error) {
	return 100, nil
}
func (p *pollingClient) Create(_ context.Context, requests []batchapi.Request) (anthropic.MessageBatch, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.requests = requests
	return anthropic.MessageBatch{ID: "batch_synthetic"}, nil
}
func (p *pollingClient) Get(context.Context, string) (anthropic.MessageBatch, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	b := anthropic.MessageBatch{ID: "batch_synthetic"}
	if p.ended {
		b.ProcessingStatus = "ended"
	}
	return b, nil
}
func (p *pollingClient) Results(context.Context, string) ([]batchapi.Result, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.downloads++
	return []batchapi.Result{{CustomID: p.requests[0].CustomID, Type: "succeeded", Message: p.message}}, nil
}
func (p *pollingClient) Cancel(context.Context, string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cancellations++
	return nil
}

func TestScreen_DatabaseCancelDuringPolling(t *testing.T) {
	db, id, rel := submissionDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	r, _, _, o := screenFixture(t)
	o.RunID, o.Release = id, rel
	client := &pollingClient{}
	if err := json.Unmarshal([]byte(`{"stop_reason":"end_turn","content":[{"type":"text","text":"{\"codes\":[\"1.1\"]}"}],"usage":{"input_tokens":100,"output_tokens":10}}`), &client.message); err != nil {
		t.Fatal(err)
	}
	r.Repository = &PGRepository{Pool: db}
	r.Client = client
	screenWaiting, screenResume := make(chan struct{}, 1), make(chan struct{})
	r.Wait = func(ctx context.Context, _ time.Duration) error {
		select {
		case screenWaiting <- struct{}{}:
		default:
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-screenResume:
			return nil
		}
	}
	screenDone := make(chan error, 1)
	go func() { _, err := r.Screen(ctx, o, documents(), families()); screenDone <- err }()
	select {
	case <-screenWaiting:
	case <-ctx.Done():
		t.Fatal("screen never entered polling")
	}
	observer := &PGRepository{Pool: db}
	unlock, err := observer.Lock(ctx)
	if err != nil {
		t.Fatal("listing blocked during screen polling", err)
	}
	rows, err := observer.List(ctx)
	unlock()
	if err != nil || len(rows) != 1 || rows[0].State != "submitted" {
		t.Fatalf("listing rows %+v error %v", rows, err)
	}
	stopper := &Runner{Repository: &PGRepository{Pool: db}, Client: client, IntentDir: r.IntentDir, Now: r.Now}
	cancelWaiting, cancelResume := make(chan struct{}, 1), make(chan struct{})
	stopper.Wait = func(ctx context.Context, _ time.Duration) error {
		select {
		case cancelWaiting <- struct{}{}:
		default:
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-cancelResume:
			return nil
		}
	}
	cancelDone := make(chan error, 1)
	go func() { cancelDone <- stopper.Cancel(ctx, rows[0].ID, o.Out) }()
	select {
	case <-cancelWaiting:
	case err = <-cancelDone:
		t.Fatal("cancellation failed during screen polling", err)
	case <-ctx.Done():
		t.Fatal("cancel never entered polling")
	}
	var state string
	if err = db.QueryRow(ctx, `SELECT state FROM submission WHERE id=$1`, rows[0].ID).Scan(&state); err != nil || state != "cancelling" {
		t.Fatalf("state %s error %v", state, err)
	}
	client.mu.Lock()
	client.ended = true
	client.mu.Unlock()
	close(screenResume)
	select {
	case err = <-screenDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("screen did not finish cancellation")
	}
	close(cancelResume)
	select {
	case err = <-cancelDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("cancel did not finish")
	}
	var cost int64
	var settled bool
	var outcomes int
	if err = db.QueryRow(ctx, `SELECT state,cost_micro_usd,cost_settled,(SELECT count(*) FROM jsonb_object_keys(results)) FROM submission WHERE id=$1`, rows[0].ID).Scan(&state, &cost, &settled, &outcomes); err != nil {
		t.Fatal(err)
	}
	if state != "cancelled" || cost != 8 || !settled || outcomes != 1 {
		t.Fatalf("state %s cost %d settled %t outcomes %d", state, cost, settled, outcomes)
	}
	client.mu.Lock()
	cancellations, downloads := client.cancellations, client.downloads
	client.mu.Unlock()
	if cancellations != 1 || downloads != 1 {
		t.Fatalf("cancellations %d downloads %d", cancellations, downloads)
	}
	if _, err := os.Stat(filepath.Join(o.Out, "families.json")); err != nil {
		t.Fatal(err)
	}
}
