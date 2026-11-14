package passwordauth

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"danny.vn/trachuan/apps/server/internal/accountflow"
	"danny.vn/trachuan/apps/server/internal/apierror"
	"danny.vn/trachuan/apps/server/internal/store"
)

func (h *Handler) PasswordLink(w http.ResponseWriter, r *http.Request) {
	p, ok := acting(r)
	if !ok {
		apierror.Write(w, 401, "unauthenticated")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if h.flowFailure(w, h.Flows.RequestAddPassword(ctx, p)) {
		return
	}
	emptyJSON(w, 202)
}
func (h *Handler) Password(w http.ResponseWriter, r *http.Request) {
	p, ok := acting(r)
	if !ok {
		apierror.Write(w, 401, "unauthenticated")
		return
	}
	var body struct {
		Token    json.RawMessage `json:"token"`
		Current  json.RawMessage `json:"current"`
		Password *string         `json:"password"`
		Code     json.RawMessage `json:"code"`
	}
	if !decode(w, r, &body) {
		return
	}
	token, tokenOK := requestString(body.Token)
	current, currentOK := requestString(body.Current)
	_, codeOK := requestString(body.Code)
	if body.Password == nil || (body.Token == nil) == (body.Current == nil) || body.Token != nil && !tokenOK || body.Current != nil && !currentOK || body.Code != nil && !codeOK {
		apierror.Write(w, 400, "invalid_request")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	account, err := h.Proofs.GetAccount(ctx, p.AccountID)
	cancel()
	if h.flowFailure(w, err) {
		return
	}
	completion := accountflow.Completion{Acting: p}
	if body.Token != nil {
		completion.TokenHash = linkDigest(token)
	}
	hash, ok := h.newPassword(w, r, *body.Password, completion.TokenHash, account.Email)
	if !ok {
		return
	}
	completion.Hash = hash
	if body.Current != nil {
		verified, match, verifyErr := h.verify(r.Context(), account.Email, current)
		if h.flowFailure(w, verifyErr) {
			return
		}
		if !match || verified.AccountID != p.AccountID || verified.AuthEpoch != p.AuthEpoch {
			apierror.Write(w, 401, "reauth_failed")
			return
		}
		completion.Verified = verified
	}
	h.commit(w, r, func(ctx context.Context, params store.CreateSessionParams) (store.CreateSessionRow, error) {
		completion.Session = params
		return h.Flows.ChangePassword(ctx, completion)
	})
}

func requestString(raw json.RawMessage) (string, bool) {
	var value string
	if len(raw) == 0 || string(raw) == "null" || json.Unmarshal(raw, &value) != nil {
		return "", false
	}
	return value, true
}
