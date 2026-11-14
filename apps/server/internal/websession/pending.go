package websession

import (
	"net/http"
	"time"

	"danny.vn/trachuan/apps/server/internal/store"
	"danny.vn/trachuan/apps/server/internal/weblogin"
)

func PendingToken(r *http.Request) (string, bool) {
	cookies := r.CookiesNamed("__Host-pending")
	if len(cookies) != 1 || !validValue(cookies[0].Value) {
		return "", false
	}
	return hashToken(cookies[0].Value), true
}
func ClearPending(w http.ResponseWriter) {
	setPending(w, "", -1)
}
func setPending(w http.ResponseWriter, token string, age int) {
	http.SetCookie(w, &http.Cookie{Name: "__Host-pending", Value: token, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: age})
}
func pendingParams(r *http.Request, next string) store.PendingParams {
	p := store.PendingParams{Next: weblogin.ValidateNext(next)}
	for _, cookie := range r.CookiesNamed("__Host-pending") {
		if validValue(cookie.Value) {
			p.ReplacedHashes = append(p.ReplacedHashes, hashToken(cookie.Value))
		}
	}
	return p
}
func pendingAge() int { return int((5 * time.Minute) / time.Second) }
