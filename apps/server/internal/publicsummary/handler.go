package publicsummary

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"danny.vn/trachuan/apps/server/internal/apierror"
)

type Handler struct {
	manager *Manager
	enabled bool
	clock   func() time.Time
	gate    chan struct{}
}

func NewHandler(manager *Manager, enabled bool, clock func() time.Time) *Handler {
	return &Handler{manager: manager, enabled: enabled, clock: clock, gate: make(chan struct{}, 8)}
}

func (h *Handler) Available(ctx context.Context, release string) (bool, error) {
	if h.manager == nil {
		return false, nil
	}
	lease, err := h.manager.Resolve(ctx)
	if err != nil {
		return false, err
	}
	defer lease.Done()
	if lease.Release.ID != release {
		return false, ErrUnavailable
	}
	return h.enabled && lease.Release.Projection == 1, nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if r.URL.RawQuery != "" || (key != "" && !validKey(key)) {
		apierror.Write(w, 400, "invalid_request")
		return
	}
	select {
	case h.gate <- struct{}{}:
		defer func() { <-h.gate }()
	default:
		writeFailure(w, ErrBusy)
		return
	}
	if h.manager == nil {
		writeFailure(w, ErrUnavailable)
		return
	}
	lease, err := h.manager.Resolve(r.Context())
	if err != nil {
		writeFailure(w, err)
		return
	}
	defer lease.Done()
	if !h.enabled || lease.Release.Projection != 1 {
		writeFailure(w, ErrNotFound)
		return
	}
	now := h.clock().In(time.FixedZone("Asia/Ho_Chi_Minh", 7*60*60))
	var raw []byte
	if key == "" {
		var items []Item
		items, err = lease.List(r.Context())
		if err == nil {
			if len(items) > 10000 {
				err = ErrTooLarge
			} else {
				if items == nil {
					items = []Item{}
				}
				raw, err = json.Marshal(listResponse{Release: lease.Release.ID, Projection: lease.Release.Projection, Total: len(items), Items: items})
				if len(raw) > 1<<20 {
					err = ErrTooLarge
				}
			}
		}
	} else {
		var row Row
		row, err = lease.Row(r.Context(), key)
		if err == nil {
			raw, err = summaryBody(row, lease.Release, now)
		}
	}
	if err != nil {
		writeFailure(w, err)
		return
	}
	midnight := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
	seconds := max(0, min(300, int(midnight.Sub(h.clock())/time.Second)))
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("ETag", fmt.Sprintf(`"%x"`, sha256.Sum256(raw)))
	w.Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d, must-revalidate", seconds))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(raw)
}

func validKey(key string) bool {
	if !strings.HasPrefix(key, "vn:") || len(key) <= 3 || !utf8.ValidString(key) || strings.ContainsAny(key, "#/?\\") {
		return false
	}
	for _, r := range key {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func writeFailure(w http.ResponseWriter, err error) {
	code, status := "unavailable", http.StatusServiceUnavailable
	switch {
	case errors.Is(err, ErrBusy):
		code = "busy"
		w.Header().Set("Retry-After", "5")
	case errors.Is(err, ErrNotFound):
		code, status = "not_found", http.StatusNotFound
	case errors.Is(err, ErrTooLarge):
		code = "too_large"
	}
	apierror.Write(w, status, code)
}
