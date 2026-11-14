package app

import (
	"net/http"

	"danny.vn/trachuan/apps/server/internal/apierror"
	"danny.vn/trachuan/apps/server/internal/passwordauth"
)

func mountFactorRoutes(mux *http.ServeMux, d webDeps) {
	h := d.password
	if h == nil {
		h = &passwordauth.Handler{PublicURL: d.publicURL}
	}
	mux.HandleFunc("GET /api/v1/auth/factor", h.FactorGet)
	mux.HandleFunc("POST /api/v1/auth/factor", h.Factor)
	for _, route := range []struct {
		pattern    string
		enrollment bool
		method     http.HandlerFunc
	}{
		{"POST /api/v1/me/totp/setup", true, h.TOTPSetup},
		{"POST /api/v1/me/totp/confirm", true, h.TOTPConfirm},
		{"DELETE /api/v1/me/totp", false, h.TOTPRemove},
		{"POST /api/v1/me/recovery-codes", false, h.RecoveryCodes},
	} {
		handler := d.sessions.Middleware(d.sessions.CSRF(route.method))
		if route.enrollment && !h.Enrollment {
			handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { apierror.Write(w, 404, "not_found") })
		}
		mux.Handle(route.pattern, handler)
	}
}
