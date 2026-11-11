package accountapi

import (
	"net/http"
	"time"

	"danny.vn/trachuan/apps/server/internal/websession"
)

type sessionResponse struct {
	ID         int64     `json:"id"`
	CreatedAt  time.Time `json:"created_at"`
	LastSeenAt time.Time `json:"last_seen_at"`
	Current    bool      `json:"current"`
}

func (h *Handler) listSessions(w http.ResponseWriter, r *http.Request) {
	p, _ := websession.FromContext(r.Context())
	rows, err := h.cfg.Store.ListSessions(r.Context(), p.AccountID)
	if err != nil || r.Context().Err() != nil {
		unavailable(w)
		return
	}
	out := make([]sessionResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, sessionResponse{row.ID, row.CreatedAt, row.LastSeenAt, row.ID == p.SessionID})
	}
	writeJSON(w, 200, out)
}
func (h *Handler) revokeSession(w http.ResponseWriter, r *http.Request) {
	p, _ := websession.FromContext(r.Context())
	id, ok := idParam(r)
	if !ok {
		notFound(w)
		return
	}
	n, err := h.cfg.Sessions.RevokeOwnSession(r.Context(), p.AccountID, id)
	if err != nil || r.Context().Err() != nil {
		unavailable(w)
		return
	}
	if n == 0 {
		notFound(w)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(204)
}
