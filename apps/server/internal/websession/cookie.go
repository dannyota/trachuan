package websession

import (
	"net/http"
	"time"
)

const sessionCookieName = "__Host-session"

func setSessionCookie(w http.ResponseWriter, raw string, expires, now time.Time) {
	maxAge := int(minTime(expires, now.Add(idleLifetime)).Sub(now) / time.Second)
	// MaxAge zero means a browser-session cookie, so never emit zero at expiry.
	if maxAge < 1 {
		maxAge = -1
		raw = ""
	}
	writeCookie(w, raw, maxAge)
}

func writeCookie(w http.ResponseWriter, raw string, maxAge int) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookieName, Value: raw, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: maxAge})
}

func sessionToken(r *http.Request) (string, bool) {
	cookies := r.CookiesNamed(sessionCookieName)
	if len(cookies) != 1 || !validValue(cookies[0].Value) {
		return "", false
	}
	return cookies[0].Value, true
}
