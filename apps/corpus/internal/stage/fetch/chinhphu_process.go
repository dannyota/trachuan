package fetch

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/chinhphu"
	"danny.vn/trachuan/apps/corpus/internal/sourcegate"
	"danny.vn/trachuan/apps/corpus/internal/stage"
)

func (s *Stage) processChinhphu(ctx context.Context, env stage.Env, run stage.Run, item stage.Item) (stage.Output, error) {
	idstr := strings.TrimPrefix(item.Key, "vn:chinhphu:")
	id, err := strconv.Atoi(idstr)
	if err != nil || id <= 0 || strconv.Itoa(id) != idstr {
		return stage.Output{}, fmt.Errorf("invalid chinhphu key %q", item.Key)
	}
	if s.chinhphu == nil {
		return stage.Output{}, fmt.Errorf("chinhphu client is required")
	}
	ctx, cancel := context.WithTimeout(ctx, s.chinhphu.ItemTimeout())
	defer cancel()
	detail, body, err := s.chinhphu.Detail(ctx, id)
	if err != nil {
		return chinhphuError(err, idstr, true)
	}
	if err = checkDetailIdentity(body, idstr); err != nil {
		return stage.Output{}, stage.SourceData(fmt.Errorf("validate detail identity: %w", err))
	}
	now := env.Now
	if now == nil {
		now = time.Now
	}
	fetched := now().UTC().Truncate(time.Microsecond)
	key, _ := json.Marshal([3]any{"detail", nil, detail.URL})
	observations := []stage.FetchObservation{{ResourceKey: string(key), Source: "chinhphu", URL: &detail.URL, Path: "detail.html", ContentSHA256: fmt.Sprintf("%x", sha256.Sum256(body)), Size: int64(len(body)), Acquisition: "network", FetchedAt: &fetched}}
	art, err := env.Artifacts.PublishContent(ctx, s.Name(), s.Recipe(run.Snapshot), func(dir string) error { return s.writeChinhphu(ctx, dir, idstr, detail, body, &observations) })
	if err != nil {
		var failure *chinhphu.Failure
		if errors.As(err, &failure) || errors.Is(err, sourcegate.ErrSourcePaused) {
			return chinhphuError(err, idstr, false)
		}
		return stage.Output{}, fmt.Errorf("publish chinhphu bundle: %w", err)
	}
	return stage.Output{Artifact: &art, State: "done", Observations: observations}, nil
}
func chinhphuError(err error, id string, detail bool) (stage.Output, error) {
	if detail && errors.Is(err, chinhphu.ErrNotFound) {
		return stage.Output{State: "gap", Gaps: []stage.Gap{{Kind: "not_found", Detail: id}}}, nil
	}
	if errors.Is(err, chinhphu.ErrTooLarge) {
		return stage.Output{State: "gap", Gaps: []stage.Gap{{Kind: "response_too_large", Detail: id}}}, nil
	}
	wrapped := fmt.Errorf("fetch chinhphu %q: %w", id, err)
	if errors.Is(err, sourcegate.ErrSourcePaused) {
		return stage.Output{}, stage.Retryable(wrapped)
	}
	var f *chinhphu.Failure
	if errors.As(err, &f) {
		if f.Origin == "source_data" {
			return stage.Output{}, stage.SourceData(wrapped)
		}
		cause := stage.GapCause{Source: "chinhphu", Origin: stage.Origin(f.Origin), Cause: stage.Cause(f.Cause), Endpoint: f.Endpoint}
		if f.HTTPStatus != 0 {
			v := int32(f.HTTPStatus)
			cause.HTTPStatus = &v
		}
		return stage.Output{}, &stage.RetryableError{Err: wrapped, GapKind: "source_unavailable", Cause: cause}
	}
	return stage.Output{}, stage.Retryable(wrapped)
}
