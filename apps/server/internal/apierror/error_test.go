package apierror

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"
)

func TestWrite_FixedMessage(t *testing.T) {
	w := httptest.NewRecorder()
	Write(w, 401, "unauthenticated")
	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if w.Code != 401 || w.Header().Get("Content-Type") != "application/json" || body.Error.Code != "unauthenticated" || body.Error.Message != "Authentication required." {
		t.Fatalf("unexpected response: %d %v %s", w.Code, w.Header(), w.Body.String())
	}
}

func TestRateLimited_RoundsUp(t *testing.T) {
	w := httptest.NewRecorder()
	RateLimited(w, 1500*time.Millisecond)
	if w.Code != 429 || w.Header().Get("Retry-After") != "2" {
		t.Fatalf("unexpected response: %d %v", w.Code, w.Header())
	}
}

func TestRetryMessage_IncludesSeconds(t *testing.T) {
	if got := RetryMessage(2); got != "Rate limited. Retry in 2 seconds." {
		t.Fatalf("message %q", got)
	}
}
