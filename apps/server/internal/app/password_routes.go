package app

import (
	"net/http"

	"danny.vn/trachuan/apps/server/internal/apierror"
)

func mountPasswordRoutes(mux *http.ServeMux, d webDeps) {
	login := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if d.password == nil {
			apierror.Write(w, 503, "unavailable")
			return
		}
		d.password.Login(w, r)
	})
	reauth := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if d.password == nil {
			apierror.Write(w, 503, "unavailable")
			return
		}
		d.password.Reauth(w, r)
	})
	mux.Handle("POST /api/v1/auth/password", d.loginLimit.middleware(login))
	mux.Handle("POST /api/v1/me/reauth", d.sessions.Middleware(d.sessions.CSRF(reauth)))
}
