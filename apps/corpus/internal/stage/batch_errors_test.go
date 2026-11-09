package stage

import (
	"context"
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
		{"canceled", context.Canceled, false},
		{"superseded", fmt.Errorf("commit: %w", ErrSuperseded), false},
	}
	for _, tt := range tests {
		if got := itemError(tt.err); got != tt.item {
			t.Errorf("%s: itemError=%v, want %v", tt.name, got, tt.item)
		}
	}
}
