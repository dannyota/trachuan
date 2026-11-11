// Package apierror defines fixed public API error messages.
package apierror

import (
	"encoding/json"
	"math"
	"net/http"
	"strconv"
	"time"
)

// Message returns the fixed public message for code.
func Message(code string) string {
	messages := map[string]string{
		"invalid_request": "Invalid request.", "unauthenticated": "Authentication required.",
		"csrf_rejected": "CSRF check failed.", "account_blocked": "Account blocked.",
		"not_found": "Not found.", "release_expired": "Release expired.",
		"body_too_large": "Request body too large.", "unsupported_media_type": "Unsupported media type.",
		"rate_limited": "Rate limited.", "busy": "Query embedder busy.",
		"unavailable": "Service unavailable.", "inference_failed": "Query inference failed.", "internal": "Internal error.",
	}
	return messages[code]
}

// Write sends a JSON error with a fixed public message.
func Write(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code, "message": Message(code)}})
}

// RateLimited sends a rate limit error and a rounded-up retry delay.
func RateLimited(w http.ResponseWriter, retryAfter time.Duration) {
	seconds := max(1, int64(math.Ceil(retryAfter.Seconds())))
	w.Header().Set("Retry-After", strconv.FormatInt(seconds, 10))
	Write(w, http.StatusTooManyRequests, "rate_limited")
}
