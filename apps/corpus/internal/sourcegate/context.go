package sourcegate

import "context"

type runIDKey struct{}
type itemKey struct{}

// WithItem attaches the document key or query identity to source requests.
func WithItem(ctx context.Context, item string) context.Context {
	return context.WithValue(ctx, itemKey{}, item)
}

// WithRunID attaches the run used for source request admission.
func WithRunID(ctx context.Context, runID int64) context.Context {
	return context.WithValue(ctx, runIDKey{}, runID)
}

// RunID returns the run attached to a context.
func RunID(ctx context.Context) (int64, bool) {
	id, ok := ctx.Value(runIDKey{}).(int64)
	return id, ok && id > 0
}
