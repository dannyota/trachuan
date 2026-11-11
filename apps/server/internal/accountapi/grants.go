package accountapi

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"time"

	"danny.vn/trachuan/apps/server/internal/store"
	"danny.vn/trachuan/apps/server/internal/websession"
	"github.com/jackc/pgx/v5"
)

type grantStore interface {
	ListGrants(context.Context, int64) ([]store.ListGrantsRow, error)
	RevokeOwnGrant(context.Context, store.RevokeOwnGrantParams) (store.RevokeOwnGrantRow, error)
}
type grantEvictor interface{ EvictGrant(int64) }
type grantResponse struct {
	ID           int64     `json:"id"`
	ClientName   string    `json:"client_name"`
	ClientIDHost string    `json:"client_id_host"`
	CreatedAt    time.Time `json:"created_at"`
	LastUsedAt   time.Time `json:"last_used_at"`
}

func (h *Handler) listGrants(w http.ResponseWriter, r *http.Request) {
	db, ok := h.cfg.Store.(grantStore)
	if !ok {
		unavailable(w)
		return
	}
	p, _ := websession.FromContext(r.Context())
	rows, err := db.ListGrants(r.Context(), p.AccountID)
	if err != nil || r.Context().Err() != nil {
		unavailable(w)
		return
	}
	out := make([]grantResponse, 0, len(rows))
	for _, row := range rows {
		host := ""
		if u, err := url.Parse(row.ClientID); err == nil && u.Scheme == "https" {
			host = u.Hostname()
		}
		out = append(out, grantResponse{row.ID, row.Name, host, row.CreatedAt, row.LastUsedAt})
	}
	writeJSON(w, 200, out)
}
func (h *Handler) revokeGrant(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(r)
	if !ok {
		notFound(w)
		return
	}
	db, ok := h.cfg.Store.(grantStore)
	if !ok {
		unavailable(w)
		return
	}
	evictor, ok := h.cfg.Verifier.(grantEvictor)
	if !ok {
		unavailable(w)
		return
	}
	p, _ := websession.FromContext(r.Context())
	_, err := db.RevokeOwnGrant(r.Context(), store.RevokeOwnGrantParams{ID: id, AccountID: p.AccountID})
	// A failed response can have an unknown commit outcome.
	if !errors.Is(err, pgx.ErrNoRows) {
		evictor.EvictGrant(id)
	}
	if err != nil {
		storeError(w, err)
		return
	}
	if r.Context().Err() != nil {
		unavailable(w)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(204)
}
