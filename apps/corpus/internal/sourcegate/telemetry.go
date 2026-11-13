package sourcegate

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync"
	"time"
)

type telemetryKey struct{}
type attemptKey struct{}
type endpointKey struct{}
type validationKey struct{}

type validationScope struct{ index int }

// ErrInvalidBody marks a response body that failed source validation.
var ErrInvalidBody = errors.New("invalid response body")

// RequestRecord holds non-identifying request measurement.
type RequestRecord struct {
	Source, Endpoint, Outcome string
	Attempt                   int
	StartedAt                 time.Time
	DurationMS                int64
	Status                    *int
}

// Telemetry collects the requests of one item.
type Telemetry struct {
	mu      sync.Mutex
	records []RequestRecord
}

// WithTelemetry starts a per-item request collector.
func WithTelemetry(ctx context.Context) (context.Context, *Telemetry) {
	t := &Telemetry{}
	return context.WithValue(ctx, telemetryKey{}, t), t
}

// Records returns a snapshot of the completed request attempts.
func (t *Telemetry) Records() []RequestRecord {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]RequestRecord(nil), t.records...)
}

// WithAttempt marks the retry number of a logical request.
func WithAttempt(ctx context.Context, attempt int) context.Context {
	return context.WithValue(ctx, attemptKey{}, attempt)
}

// WithEndpoint marks a fixed endpoint category without retaining request data.
func WithEndpoint(ctx context.Context, endpoint string) context.Context {
	return context.WithValue(ctx, endpointKey{}, endpoint)
}

// Endpoint returns the fixed endpoint category for an item request.
func Endpoint(ctx context.Context) string {
	value, _ := ctx.Value(endpointKey{}).(string)
	return value
}

// WithBodyValidation lets a client classify a body after parsing it.
func WithBodyValidation(ctx context.Context) (context.Context, func(bool)) {
	scope := &validationScope{index: -1}
	t, _ := ctx.Value(telemetryKey{}).(*Telemetry)
	finish := func(invalid bool) {
		if !invalid || t == nil {
			return
		}
		t.mu.Lock()
		defer t.mu.Unlock()
		if scope.index >= 0 {
			row := &t.records[scope.index]
			if row.Status != nil && *row.Status >= 200 && *row.Status < 300 && row.Outcome == "ok" {
				row.Outcome = "invalid_body"
			}
		}
	}
	return context.WithValue(ctx, validationKey{}, scope), finish
}

// RecordAttempt appends a request that reached the HTTP transport.
func RecordAttempt(ctx context.Context, source, endpoint string, started time.Time, status int, err error) {
	t, _ := ctx.Value(telemetryKey{}).(*Telemetry)
	if t == nil || endpoint == "" {
		return
	}
	attempt, _ := ctx.Value(attemptKey{}).(int)
	if attempt < 1 {
		attempt = 1
	}
	row := RequestRecord{Source: source, Endpoint: endpoint, Attempt: attempt, StartedAt: started.UTC(), DurationMS: time.Since(started).Milliseconds()}
	if status != 0 {
		row.Status = &status
	}
	var n net.Error
	switch {
	case errors.Is(err, ErrSourcePaused):
		row.Outcome = "paused"
	case errors.Is(err, context.Canceled):
		row.Outcome = "cancelled"
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &n) && n.Timeout():
		row.Outcome = "timeout"
	case status >= 200 && status < 300 && errors.Is(err, ErrInvalidBody):
		row.Outcome = "invalid_body"
	case status != 0 && status != http.StatusOK:
		row.Outcome = "http_error"
	case err != nil:
		row.Outcome = "transport"
	default:
		row.Outcome = "ok"
	}
	t.mu.Lock()
	if scope, ok := ctx.Value(validationKey{}).(*validationScope); ok {
		scope.index = len(t.records)
	}
	t.records = append(t.records, row)
	t.mu.Unlock()
}
