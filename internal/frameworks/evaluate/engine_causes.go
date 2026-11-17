package evaluate

import (
	"context"
	"errors"
	"sync"

	"danny.vn/trachuan/internal/frameworks/serve"
	"github.com/jackc/pgx/v5"
)

type causeKey struct{}
type operationCause struct {
	mu  sync.Mutex
	err error
}
type CauseTracer struct{}

func (CauseTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	return ctx
}
func (CauseTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	recordCause(ctx, data.Err)
}
func (CauseTracer) TraceConnectStart(ctx context.Context, _ pgx.TraceConnectStartData) context.Context {
	return ctx
}
func (CauseTracer) TraceConnectEnd(ctx context.Context, data pgx.TraceConnectEndData) {
	recordCause(ctx, data.Err)
}
func recordCause(ctx context.Context, err error) {
	if err == nil {
		return
	}
	if c, ok := ctx.Value(causeKey{}).(*operationCause); ok {
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.err == nil {
			c.err = err
		}
	}
}

type engineCause struct{ original, cause error }

func (e *engineCause) Error() string        { return "evaluation engine operation failed" }
func (e *engineCause) Unwrap() error        { return e.cause }
func (e *engineCause) Is(target error) bool { return errors.Is(e.original, target) }
func captureOperation[T any](ctx context.Context, operation func(context.Context) (T, error)) (T, error) {
	cause := &operationCause{}
	ctx = context.WithValue(ctx, causeKey{}, cause)
	value, err := operation(ctx)
	cause.mu.Lock()
	defer cause.mu.Unlock()
	if err != nil && cause.err != nil {
		err = &engineCause{original: err, cause: cause.err}
	}
	return value, err
}

type causeEngine struct{ Engine }

func WithFailureCauses(e Engine) Engine { return causeEngine{e} }
func (e causeEngine) Search(ctx context.Context, r serve.Request, c serve.Caller) (serve.Response, error) {
	return captureOperation(ctx, func(ctx context.Context) (serve.Response, error) { return e.Engine.Search(ctx, r, c) })
}
func (e causeEngine) Read(ctx context.Context, r serve.ReadRequest, c serve.Caller) (serve.ReadResponse, error) {
	return captureOperation(ctx, func(ctx context.Context) (serve.ReadResponse, error) { return e.Engine.Read(ctx, r, c) })
}
func (e causeEngine) Mappings(ctx context.Context, r serve.MappingsRequest, c serve.Caller) (serve.MappingsResponse, error) {
	return captureOperation(ctx, func(ctx context.Context) (serve.MappingsResponse, error) { return e.Engine.Mappings(ctx, r, c) })
}
func (e causeEngine) CorpusStatus(ctx context.Context, r serve.StatusRequest, c serve.Caller) (serve.StatusResponse, error) {
	return captureOperation(ctx, func(ctx context.Context) (serve.StatusResponse, error) { return e.Engine.CorpusStatus(ctx, r, c) })
}
func (e causeEngine) QualityGaps(ctx context.Context, r serve.GapsRequest, c serve.Caller) (serve.GapsResponse, error) {
	return captureOperation(ctx, func(ctx context.Context) (serve.GapsResponse, error) { return e.Engine.QualityGaps(ctx, r, c) })
}
func (e causeEngine) CalibrationSearch(ctx context.Context, q string) (*float64, bool, error) {
	type result struct {
		score  *float64
		lookup bool
	}
	value, err := captureOperation(ctx, func(ctx context.Context) (result, error) {
		s, l, err := e.Engine.CalibrationSearch(ctx, q)
		return result{s, l}, err
	})
	return value.score, value.lookup, err
}
func OpenWithFailureCauses(ctx context.Context, open func(context.Context) (Engine, error)) (Engine, error) {
	return captureOperation(ctx, open)
}
