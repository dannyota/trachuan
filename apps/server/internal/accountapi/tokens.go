package accountapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"danny.vn/trachuan/apps/server/internal/apierror"
	"danny.vn/trachuan/apps/server/internal/credential"
	"danny.vn/trachuan/apps/server/internal/store"
	"danny.vn/trachuan/apps/server/internal/websession"
)

type tokenResponse struct {
	ID         int64      `json:"id"`
	Name       string     `json:"name"`
	CreatedAt  time.Time  `json:"created_at"`
	ExpiresAt  time.Time  `json:"expires_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	RevokedAt  *time.Time `json:"revoked_at"`
}

func tokenView(t store.PersonalToken) tokenResponse {
	return tokenResponse{t.ID, t.Name, t.CreatedAt, t.ExpiresAt, t.LastUsedAt, t.RevokedAt}
}
func (h *Handler) listTokens(w http.ResponseWriter, r *http.Request) {
	p, _ := websession.FromContext(r.Context())
	rows, err := h.cfg.Store.ListTokens(r.Context(), p.AccountID)
	if err != nil || r.Context().Err() != nil {
		unavailable(w)
		return
	}
	out := make([]tokenResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, tokenView(row))
	}
	writeJSON(w, 200, out)
}
func (h *Handler) createToken(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	var body struct {
		Name string `json:"name"`
		Days *int   `json:"days"`
	}
	err := dec.Decode(&body)
	if err == nil {
		var extra any
		err = dec.Decode(&extra)
		if errors.Is(err, io.EOF) {
			err = nil
		} else if err == nil {
			invalid(w)
			return
		}
	}
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) {
		apierror.Write(w, 413, "body_too_large")
		return
	}
	if err != nil {
		invalid(w)
		return
	}
	name := strings.TrimSpace(body.Name)
	days := 90
	if body.Days != nil {
		days = *body.Days
	}
	if !utf8.ValidString(name) || utf8.RuneCountInString(name) < 1 || utf8.RuneCountInString(name) > 64 || strings.IndexFunc(name, unicode.IsControl) >= 0 || days < 1 || days > 365 {
		invalid(w)
		return
	}
	p, _ := websession.FromContext(r.Context())
	a, err := h.cfg.Store.GetAccount(r.Context(), p.AccountID)
	if err != nil || r.Context().Err() != nil {
		unavailable(w)
		return
	}
	h.randomMu.Lock()
	plain, hash, err := credential.Issue(h.cfg.Rand)
	h.randomMu.Unlock()
	if err != nil {
		unavailable(w)
		return
	}
	row, err := h.cfg.Store.CreatePersonalToken(r.Context(), store.CreatePersonalTokenParams{TokenHash: hash, AccountID: p.AccountID, Name: name, ExpiresAt: h.cfg.Clock().Add(time.Duration(days) * 24 * time.Hour), AuthEpoch: a.AuthEpoch})
	if errors.Is(err, credential.ErrUnauthorized) && r.Context().Err() == nil {
		apierror.Write(w, 403, "account_blocked")
		return
	}
	if errors.Is(err, credential.ErrTokenLimit) && r.Context().Err() == nil {
		apierror.Write(w, 409, "token_limit")
		return
	}
	if err != nil || r.Context().Err() != nil {
		unavailable(w)
		return
	}
	writeJSON(w, 201, struct {
		tokenResponse
		Token string `json:"token"`
	}{tokenView(row), plain})
}
func (h *Handler) revokeToken(w http.ResponseWriter, r *http.Request) {
	p, _ := websession.FromContext(r.Context())
	id, ok := idParam(r)
	if !ok {
		notFound(w)
		return
	}
	digest, err := h.cfg.Store.RevokeOwnToken(r.Context(), store.RevokeOwnTokenParams{AccountID: p.AccountID, ID: id})
	if err != nil || r.Context().Err() != nil {
		// A failed response can hide a committed revocation.
		h.cfg.Verifier.EvictAccount(p.AccountID)
		if r.Context().Err() != nil {
			unavailable(w)
		} else {
			storeError(w, err)
		}
		return
	}
	h.cfg.Verifier.Evict(digest)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(204)
}
