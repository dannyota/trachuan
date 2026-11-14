package passwordauth

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"danny.vn/trachuan/apps/server/internal/accountflow"
	"danny.vn/trachuan/apps/server/internal/apierror"
	"danny.vn/trachuan/apps/server/internal/factor"
	"danny.vn/trachuan/apps/server/internal/store"
	"danny.vn/trachuan/apps/server/internal/weblogin"
	"danny.vn/trachuan/apps/server/internal/websession"
)

type FactorStore interface {
	PendingFactor(context.Context, string) (store.PendingAuth, error)
	CompleteFactor(context.Context, accountflow.FactorCompletion) (store.CreateSessionRow, string, error)
	SetupTOTP(context.Context, accountflow.Acting) (accountflow.TOTPSetup, error)
	ConfirmTOTP(context.Context, accountflow.FactorChange) (store.CreateSessionRow, []string, error)
	RemoveTOTP(context.Context, accountflow.FactorChange) (store.CreateSessionRow, error)
	ReplaceRecoveryCodes(context.Context, accountflow.FactorChange) (store.CreateSessionRow, []string, error)
}

func factorJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(value)
}
func (h *Handler) pending(w http.ResponseWriter, r *http.Request) (store.PendingAuth, string, bool) {
	hash, ok := websession.PendingToken(r)
	if !ok {
		websession.ClearPending(w)
		apierror.Write(w, 401, "factor_failed")
		return store.PendingAuth{}, "", false
	}
	if h.Factors == nil {
		apierror.Write(w, 503, "unavailable")
		return store.PendingAuth{}, "", false
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	row, err := h.Factors.PendingFactor(ctx, hash)
	if errors.Is(err, factor.ErrFailed) {
		websession.ClearPending(w)
	}
	if h.flowFailure(w, err) {
		return row, "", false
	}
	return row, hash, true
}
func (h *Handler) FactorGet(w http.ResponseWriter, r *http.Request) {
	row, _, ok := h.pending(w, r)
	if !ok {
		return
	}
	next := "/"
	if row.Next != nil {
		next = weblogin.ValidateNext(*row.Next)
	}
	factorJSON(w, map[string]string{"csrf": row.CsrfSecret, "next": next})
}

type factorBody struct {
	Code     json.RawMessage `json:"code"`
	Recovery json.RawMessage `json:"recovery_code"`
}

func readFactor(w http.ResponseWriter, r *http.Request, allowRecovery bool) (string, string, bool, bool) {
	var body factorBody
	if !decode(w, r, &body) {
		return "", "", false, false
	}
	code, codeOK := requestString(body.Code)
	recovery, recoveryOK := requestString(body.Recovery)
	if (body.Code == nil) == (body.Recovery == nil) || body.Code != nil && !codeOK || body.Recovery != nil && (!allowRecovery || !recoveryOK) {
		apierror.Write(w, 400, "invalid_request")
		return "", "", false, false
	}
	return code, recovery, body.Recovery != nil, true
}
func (h *Handler) Factor(w http.ResponseWriter, r *http.Request) {
	if !h.preSession(w, r) {
		return
	}
	row, hash, ok := h.pending(w, r)
	if !ok {
		return
	}
	tokens := r.Header.Values("X-CSRF-Token")
	if len(tokens) != 1 || len(row.CsrfSecret) != 43 || subtle.ConstantTimeCompare([]byte(tokens[0]), []byte(row.CsrfSecret)) != 1 {
		apierror.Write(w, 403, "csrf_rejected")
		return
	}
	code, recovery, useRecovery, ok := readFactor(w, r, true)
	if !ok {
		return
	}
	h.factorCommit(w, r, func(ctx context.Context, p store.CreateSessionParams) (store.CreateSessionRow, any, error) {
		session, path, err := h.Factors.CompleteFactor(ctx, accountflow.FactorCompletion{TokenHash: hash, CSRF: row.CsrfSecret, Code: code, RecoveryCode: recovery, UseRecovery: useRecovery, Session: p})
		next := weblogin.ValidateNext(path)
		return session, map[string]string{"next": next}, err
	}, true)
}
func (h *Handler) factorCommit(w http.ResponseWriter, r *http.Request, write func(context.Context, store.CreateSessionParams) (store.CreateSessionRow, any, error), pending bool) {
	sessions, ok := h.Sessions.(flowSessions)
	if !ok {
		apierror.Write(w, 503, "unavailable")
		return
	}
	var body any
	err := sessions.CommitSession(r.Context(), w, r, func(ctx context.Context, p store.CreateSessionParams) (store.CreateSessionRow, error) {
		row, value, err := write(ctx, p)
		if row.AccountID != 0 {
			h.evict(row.AccountID)
		}
		body = value
		return row, err
	})
	if pending && errors.Is(err, factor.ErrFailed) {
		hash, valid := websession.PendingToken(r)
		if valid {
			ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
			_, pendingErr := h.Factors.PendingFactor(ctx, hash)
			cancel()
			if errors.Is(pendingErr, factor.ErrFailed) {
				websession.ClearPending(w)
			}
		}
	}
	if h.flowFailure(w, err) {
		return
	}
	if pending {
		websession.ClearPending(w)
	}
	factorJSON(w, body)
}
