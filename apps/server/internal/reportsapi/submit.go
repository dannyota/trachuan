package reportsapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"danny.vn/trachuan/apps/server/internal/apierror"
	"danny.vn/trachuan/apps/server/internal/reports"
	"danny.vn/trachuan/apps/server/internal/websession"
)

func (h *handler) submit(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
	raw, err := io.ReadAll(r.Body)
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) {
		apierror.Write(w, 413, "body_too_large")
		return
	}
	var in reports.Input
	if err != nil || json.Unmarshal(raw, &in) != nil {
		apierror.Write(w, 400, "invalid_request")
		return
	}
	if h.cfg.Reports == nil {
		apierror.Write(w, 503, "unavailable")
		return
	}
	p, _ := websession.FromContext(r.Context())
	result, err := h.cfg.Reports.Submit(r.Context(), p.AccountID, in)
	if err != nil || r.Context().Err() != nil {
		switch {
		case r.Context().Err() != nil:
			apierror.Write(w, 503, "unavailable")
		case errors.Is(err, reports.ErrInvalidRequest):
			apierror.Write(w, 400, "invalid_request")
		case errors.Is(err, reports.ErrUnauthenticated):
			apierror.Write(w, 401, "unauthenticated")
		case errors.Is(err, reports.ErrReleaseExpired):
			apierror.Write(w, 409, "release_expired")
		case errors.Is(err, reports.ErrRateLimited):
			apierror.RateLimited(w, time.Second)
		default:
			apierror.Write(w, 503, "unavailable")
		}
		return
	}
	writeJSON(w, result)
}
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(v)
}
