package accountapi

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"danny.vn/trachuan/apps/server/internal/websession"
)

// PurgeStore deletes accounts whose deletion grace period has elapsed.
type PurgeStore interface {
	PurgeDeletedAccounts(context.Context, time.Time) (int64, error)
}

// Purge deletes eligible accounts at the given time. Call it hourly.
func Purge(ctx context.Context, s PurgeStore, now time.Time) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	n, err := s.PurgeDeletedAccounts(ctx, now)
	if err != nil {
		return 0, fmt.Errorf("purge deleted accounts: %w", err)
	}
	return n, nil
}
func (h *Handler) deleteAccount(w http.ResponseWriter, r *http.Request) {
	p, _ := websession.FromContext(r.Context())
	_, err := h.cfg.Store.RequestDeletion(r.Context(), p.AccountID)
	h.cfg.Verifier.EvictAccount(p.AccountID)
	h.cfg.Sessions.EvictAccount(p.AccountID)
	if err != nil || r.Context().Err() != nil {
		if err != nil {
			storeError(w, err)
		} else {
			unavailable(w)
		}
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(204)
}
