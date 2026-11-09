package stage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestItemError_Classifies(t *testing.T) {
	_, missing := os.ReadFile("/definitely/not/here")
	tests := []struct {
		name string
		err  error
		item bool
	}{
		{"missing local file", fmt.Errorf("load rows: %w", missing), true},
		{"data exception", &pgconn.PgError{Code: "22021"}, true},
		{"integrity violation", &pgconn.PgError{Code: "23505"}, true},
		{"server shutdown", &pgconn.PgError{Code: "57P01"}, false},
		{"network", &net.OpError{Op: "dial", Err: errors.New("refused")}, false},
		{"timeout inside one item", fmt.Errorf("download: %w", context.DeadlineExceeded), true},
		{"superseded", fmt.Errorf("commit: %w", ErrSuperseded), false},
	}
	for _, tt := range tests {
		if got := itemError(context.Background(), tt.err); got != tt.item {
			t.Errorf("%s: itemError=%v, want %v", tt.name, got, tt.item)
		}
	}
}

func TestItemError_DoneActivityContextAbortsBatch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if itemError(ctx, errors.New("anything")) {
		t.Fatal("an error after the activity context ended must abort the batch")
	}
}

func TestSourceDataError_Classifies(t *testing.T) {
	var syntax *json.SyntaxError
	err := json.Unmarshal([]byte("{"), &struct{}{})
	if !errors.As(err, &syntax) {
		t.Fatal("expected a syntax error to test with")
	}
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"broken JSON in a source payload", fmt.Errorf("read detail: %w", err), true},
		{"NUL in text", &pgconn.PgError{Code: "22021"}, true},
		{"duplicate node id", &pgconn.PgError{Code: "23505"}, true},
		{"missing local file", fmt.Errorf("read: %w", os.ErrNotExist), false},
		{"network", &net.OpError{Op: "dial", Err: errors.New("refused")}, false},
	}
	for _, tt := range tests {
		if got := sourceDataError(tt.err); got != tt.want {
			t.Errorf("%s: sourceDataError=%v, want %v", tt.name, got, tt.want)
		}
	}
}
