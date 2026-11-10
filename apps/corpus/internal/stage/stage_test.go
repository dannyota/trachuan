package stage

import (
	"errors"
	"fmt"
	"testing"
)

func TestRetryable_WrappedError(t *testing.T) {
	cause := errors.New("temporary failure")
	err := fmt.Errorf("process item: %w", Retryable(cause))
	if !errors.Is(err, cause) {
		t.Fatal("retryable error lost its cause")
	}
	var retryable *RetryableError
	if !errors.As(err, &retryable) {
		t.Fatal("retryable error lost its classification")
	}
}

func TestRetryableGap_CarriesCause(t *testing.T) {
	err := &RetryableError{Err: errors.New("gateway failure"), GapKind: "invalid_source_data", Cause: GapCause{Origin: OriginGateway, Cause: CauseHTTP5xx}}
	out := processFailure(err, err)
	if len(out.Gaps) != 1 || out.Gaps[0].Origin != OriginGateway || out.Gaps[0].Cause != CauseHTTP5xx {
		t.Fatalf("lost cause: %+v", out)
	}
}

func TestRetryEligible_StructuredCausesOnly(t *testing.T) {
	tests := []struct {
		gap  Gap
		want bool
	}{
		{Gap{Kind: "source_unavailable"}, true},
		{Gap{Kind: "invalid_source_data", Origin: OriginGateway, Cause: CauseNonJSON200}, true},
		{Gap{Kind: "invalid_source_data", Origin: OriginGateway, Cause: CauseHTTP5xx}, true},
		{Gap{Kind: "invalid_source_data", Detail: "gateway non_json_200"}, false},
		{Gap{Kind: "invalid_source_data", Origin: OriginSourceData, Cause: CauseHTTP5xx}, false},
	}
	for _, tt := range tests {
		if got := RetryEligible(tt.gap); got != tt.want {
			t.Errorf("%+v: got %v", tt.gap, got)
		}
	}
}

func TestGap_RejectsInvalidEnums(t *testing.T) {
	for _, gap := range []Gap{{Origin: "bad"}, {Cause: "bad"}} {
		if gap.Validate() == nil {
			t.Errorf("accepted %+v", gap)
		}
	}
}

func TestSourceData_Origin(t *testing.T) {
	out := processFailure(SourceData(errors.New("invalid payload")), nil)
	if len(out.Gaps) != 1 || out.Gaps[0].Origin != OriginSourceData {
		t.Fatalf("missing source data origin: %+v", out)
	}
}

func TestRetryableSourceData_DefaultOrigin(t *testing.T) {
	err := &RetryableError{Err: SourceData(errors.New("invalid")), GapKind: "invalid_source_data"}
	out := processFailure(err, err)
	if out.Gaps[0].Origin != OriginSourceData {
		t.Fatalf("origin: %q", out.Gaps[0].Origin)
	}
}
