package reportsapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"danny.vn/trachuan/apps/server/internal/apierror"
	"danny.vn/trachuan/apps/server/internal/store"
	"danny.vn/trachuan/apps/server/internal/websession"
)

type reportView struct {
	ReportID       int64           `json:"report_id"`
	Kind           string          `json:"kind"`
	DocumentKey    string          `json:"document_key"`
	Anchor         string          `json:"anchor"`
	DocumentNumber string          `json:"document_number"`
	Target         string          `json:"target"`
	Suggestion     json.RawMessage `json:"suggestion"`
	State          string          `json:"state"`
	CreatedAt      time.Time       `json:"created_at"`
	ChangedAt      time.Time       `json:"changed_at"`
}

func (h *handler) list(w http.ResponseWriter, r *http.Request) {
	var at *time.Time
	var id *int64
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		apierror.Write(w, 400, "invalid_request")
		return
	}
	if values, ok := query["next"]; ok {
		if len(values) != 1 {
			apierror.Write(w, 400, "invalid_request")
			return
		}
		t, n, valid := decodeCursor(values[0])
		if !valid {
			apierror.Write(w, 400, "invalid_request")
			return
		}
		at = &t
		id = &n
	}
	if h.cfg.Store == nil {
		apierror.Write(w, 503, "unavailable")
		return
	}
	p, _ := websession.FromContext(r.Context())
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	rows, err := h.cfg.Store.ListOwnReports(ctx, store.ListOwnReportsParams{AccountID: p.AccountID, CursorCreatedAt: at, CursorID: id, PageSize: 101})
	if err != nil || ctx.Err() != nil {
		apierror.Write(w, 503, "unavailable")
		return
	}
	next := ""
	if len(rows) > 100 {
		rows = rows[:100]
		last := rows[len(rows)-1]
		next = encodeCursor(last.CreatedAt, last.ID)
	}
	views := make([]reportView, 0, len(rows))
	for _, row := range rows {
		views = append(views, reportView{row.ID, row.Kind, row.DocumentKey, row.Anchor, row.DocumentNumber, row.Target, row.Suggestion, row.State, row.CreatedAt, row.ChangedAt})
	}
	writeJSON(w, struct {
		Reports []reportView `json:"reports"`
		Next    string       `json:"next,omitempty"`
	}{views, next})
}
func (h *handler) withdraw(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		apierror.Write(w, 404, "not_found")
		return
	}
	if h.cfg.Store == nil {
		apierror.Write(w, 503, "unavailable")
		return
	}
	p, _ := websession.FromContext(r.Context())
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	n, err := h.cfg.Store.WithdrawOwnReport(ctx, store.WithdrawOwnReportParams{ID: id, AccountID: p.AccountID})
	if err != nil || ctx.Err() != nil {
		apierror.Write(w, 503, "unavailable")
		return
	}
	if n == 0 {
		apierror.Write(w, 404, "not_found")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(204)
}
