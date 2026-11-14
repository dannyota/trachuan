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
	signupLimit, resetLimit := newMailLimiter(d.loginLimit), newMailLimiter(d.loginLimit)
	for path, method := range map[string]func(http.ResponseWriter, *http.Request){
		"signup": nil, "reset": nil, "link": nil, "signup/complete": nil, "reset/complete": nil,
	} {
		if d.password != nil && d.password.Flows != nil {
			switch path {
			case "signup":
				method = d.password.Signup
			case "reset":
				method = d.password.Reset
			case "link":
				method = d.password.Link
			case "signup/complete":
				method = d.password.SignupComplete
			case "reset/complete":
				method = d.password.ResetComplete
			}
		}
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if method == nil {
				apierror.Write(w, 503, "unavailable")
				return
			}
			method(w, r)
		})
		limited := d.loginLimit.middleware(handler)
		if path == "signup" {
			limited = signupLimit.middleware(handler)
		}
		if path == "reset" {
			limited = resetLimit.middleware(handler)
		}
		mux.Handle("POST /api/v1/auth/"+path, limited)
	}
	for path, link := range map[string]bool{"/api/v1/me/password": false, "/api/v1/me/password/link": true} {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if d.password == nil || d.password.Flows == nil {
				apierror.Write(w, 503, "unavailable")
				return
			}
			if link {
				d.password.PasswordLink(w, r)
			} else {
				d.password.Password(w, r)
			}
		})
		mux.Handle("POST "+path, d.sessions.Middleware(d.sessions.CSRF(handler)))
	}
	mux.Handle("POST /api/v1/auth/password", d.loginLimit.middleware(login))
	mux.Handle("POST /api/v1/me/reauth", d.sessions.Middleware(d.sessions.CSRF(reauth)))
}
