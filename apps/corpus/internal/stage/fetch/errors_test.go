package fetch

import (
	"encoding/json"
	"errors"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/vbpl"
)

func TestAPIError_Classifies(t *testing.T) {
	_, err := apiError(&json.SyntaxError{}, "1", true)
	if !errors.Is(err, stage.ErrSourceData) {
		t.Fatalf("malformed payload: %v", err)
	}
	out, err := apiError(&vbpl.StatusError{Code: 500}, "21657", true)
	var retryable *stage.RetryableError
	if err != nil || out.State != "gap" || len(out.Gaps) != 1 || out.Gaps[0].Kind != "source_unavailable" || out.Gaps[0].Detail != "21657" || out.Gaps[0].Cause != stage.CauseHTTP5xx || out.Gaps[0].Origin != stage.OriginGateway {
		t.Fatalf("persistent 500: %+v, %v", out, err)
	}
	_, err = apiError(errors.New("connection reset"), "1", true)
	if !errors.As(err, &retryable) || retryable.GapKind != "" {
		t.Fatalf("network error must stay a plain retry: %v", err)
	}
}
