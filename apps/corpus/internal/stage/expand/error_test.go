package expand

import (
	"encoding/json"
	"errors"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/sourcegate"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/vbpl"
)

func TestExpand_ErrorMapping(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		detail bool
		gap    string
	}{{"too large", vbpl.ErrTooLarge, false, "response_too_large"}, {"not found", vbpl.ErrNotFound, true, "not_found"}} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := apiError(tc.err, "1", tc.detail)
			if err != nil || len(out.Gaps) != 1 || out.Gaps[0].Kind != tc.gap {
				t.Fatalf("output=%#v err=%v", out, err)
			}
		})
	}
	_, err := apiError(&json.SyntaxError{}, "1", true)
	if !errors.Is(err, stage.ErrSourceData) {
		t.Fatalf("syntax error=%v", err)
	}
	_, err = apiError(sourcegate.ErrSourcePaused, "1", false)
	var retry *stage.RetryableError
	if !errors.As(err, &retry) || !errors.Is(err, sourcegate.ErrSourcePaused) {
		t.Fatalf("pause error=%v", err)
	}
	failure := &vbpl.Failure{Origin: "gateway", Cause: "http_429", Endpoint: "diagram", HTTPStatus: 429}
	_, err = apiError(failure, "1", false)
	if !errors.As(err, &retry) || retry.GapKind != "source_unavailable" || retry.Cause.Origin != stage.OriginGateway || retry.Cause.Cause != stage.CauseHTTP429 || retry.Cause.Endpoint != "diagram" || retry.Cause.HTTPStatus == nil || *retry.Cause.HTTPStatus != 429 {
		t.Fatalf("classified error=%#v", err)
	}
}
