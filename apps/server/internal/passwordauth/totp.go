package passwordauth

import (
	"context"
	"net/http"
	"time"

	"danny.vn/trachuan/apps/server/internal/accountflow"
	"danny.vn/trachuan/apps/server/internal/apierror"
	"danny.vn/trachuan/apps/server/internal/store"
)

func (h *Handler) TOTPSetup(w http.ResponseWriter, r *http.Request) {
	if !h.Enrollment {
		apierror.Write(w, 404, "not_found")
		return
	}
	p, ok := acting(r)
	if !ok {
		apierror.Write(w, 401, "unauthenticated")
		return
	}
	if h.Factors == nil {
		apierror.Write(w, 503, "unavailable")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	setup, err := h.Factors.SetupTOTP(ctx, p)
	if h.flowFailure(w, err) {
		return
	}
	factorJSON(w, setup)
}
func (h *Handler) TOTPConfirm(w http.ResponseWriter, r *http.Request) {
	if !h.Enrollment {
		apierror.Write(w, 404, "not_found")
		return
	}
	h.factorChange(w, r, "confirm")
}
func (h *Handler) TOTPRemove(w http.ResponseWriter, r *http.Request) { h.factorChange(w, r, "remove") }
func (h *Handler) RecoveryCodes(w http.ResponseWriter, r *http.Request) {
	h.factorChange(w, r, "recovery")
}
func (h *Handler) factorChange(w http.ResponseWriter, r *http.Request, kind string) {
	acting, ok := acting(r)
	if !ok {
		apierror.Write(w, 401, "unauthenticated")
		return
	}
	if h.Factors == nil {
		apierror.Write(w, 503, "unavailable")
		return
	}
	code, recovery, useRecovery, ok := readFactor(w, r, kind == "remove")
	if !ok {
		return
	}
	h.factorCommit(w, r, func(ctx context.Context, params store.CreateSessionParams) (store.CreateSessionRow, any, error) {
		p := accountflow.FactorChange{Acting: acting, Code: code, RecoveryCode: recovery, UseRecovery: useRecovery, Session: params}
		var row store.CreateSessionRow
		var codes []string
		var err error
		switch kind {
		case "confirm":
			row, codes, err = h.Factors.ConfirmTOTP(ctx, p)
		case "remove":
			row, err = h.Factors.RemoveTOTP(ctx, p)
		default:
			row, codes, err = h.Factors.ReplaceRecoveryCodes(ctx, p)
		}
		result := map[string]any{}
		if codes != nil {
			result["recovery_codes"] = codes
		}
		return row, result, err
	}, false)
}
