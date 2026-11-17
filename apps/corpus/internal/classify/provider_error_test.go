package classify

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/v2"
)

func TestProviderError_KeepsFixedShapeSummary(t *testing.T) {
	api := &anthropic.Error{StatusCode: 400, RequestID: "req_011ABCdef"}
	body := `{"type":"error","error":{"type":"invalid_request_error","message":"secret-looking text sk-ant-xyz echoed","details":{"error_code":"usage_limit_reached"}}}`
	if err := api.UnmarshalJSON([]byte(body)); err != nil {
		t.Fatal(err)
	}
	got := providerError(api).Error()
	for _, want := range []string{"status=400", "type=invalid_request_error", "code=usage_limit_reached", "request_id=req_011ABCdef"} {
		if !strings.Contains(got, want) {
			t.Fatalf("summary %q lacks %q", got, want)
		}
	}
	for _, banned := range []string{"sk-ant", "secret-looking", "echoed"} {
		if strings.Contains(got, banned) {
			t.Fatalf("summary %q carries free text %q", got, banned)
		}
	}
}

func TestProviderError_RejectsUnsafeFields(t *testing.T) {
	api := &anthropic.Error{StatusCode: 500, RequestID: "req bad\nline"}
	if err := api.UnmarshalJSON([]byte(`{"error":{"type":"Weird Type!","details":{"error_code":"x y"}}}`)); err != nil {
		t.Fatal(err)
	}
	got := providerError(api).Error()
	if !strings.Contains(got, "status=500") || !strings.Contains(got, "type=unknown") || !strings.Contains(got, "code=none") || !strings.Contains(got, "request_id=unknown") {
		t.Fatalf("unsafe fields not replaced: %q", got)
	}
}

func TestProviderError_KeepsContextErrors(t *testing.T) {
	if !errors.Is(providerError(context.Canceled), context.Canceled) || !errors.Is(providerError(context.DeadlineExceeded), context.DeadlineExceeded) {
		t.Fatal("context errors lost")
	}
}

func TestScreen_CreateFailureKeepsProviderSummary(t *testing.T) {
	r, ledger, client, o := screenFixture(t)
	client.beforeCreate = nil
	api := &anthropic.Error{StatusCode: 400, RequestID: "req_011Create"}
	if err := api.UnmarshalJSON([]byte(`{"error":{"type":"invalid_request_error","message":"free text","details":{"error_code":"usage_limit_reached"}}}`)); err != nil {
		t.Fatal(err)
	}
	client.createErr = api
	_, err := r.Screen(context.Background(), o, documents(), families())
	if err == nil {
		t.Fatal("expected uncertain submission")
	}
	for _, want := range []string{"reconcile submission", "status=400", "code=usage_limit_reached", "request_id=req_011Create"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("create error %q lacks %q", err, want)
		}
	}
	if client.creates != 1 {
		t.Fatalf("create calls %d", client.creates)
	}
	if strings.Contains(err.Error(), "free text") || ledger.rows[0].State != "uncertain" {
		t.Fatalf("create failure state %q error %q", ledger.rows[0].State, err)
	}
}
