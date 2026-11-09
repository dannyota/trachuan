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
	_, err = apiError(&vbpl.StatusError{Code: 500}, "21657", true)
	var retryable *stage.RetryableError
	if !errors.As(err, &retryable) || retryable.GapKind != "source_unavailable" {
		t.Fatalf("persistent 500: %v", err)
	}
	_, err = apiError(errors.New("connection reset"), "1", true)
	if !errors.As(err, &retryable) || retryable.GapKind != "" {
		t.Fatalf("network error must stay a plain retry: %v", err)
	}
}
