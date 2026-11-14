package websession

import (
	"context"
	"net/http"

	"danny.vn/trachuan/apps/server/internal/store"
)

type passwordSessionStore interface {
	CreatePasswordSession(context.Context, store.PasswordSessionParams) (store.CreateSessionRow, error)
}

func (m *Manager) CreatePasswordSession(ctx context.Context, w http.ResponseWriter, r *http.Request, verified store.PasswordSnapshot) error {
	ctx, cancel := context.WithTimeout(ctx, authQueryDeadline)
	defer cancel()
	db, ok := m.Store.(passwordSessionStore)
	if !ok {
		return errUnavailable
	}
	token, secret, err := m.newValues()
	if err != nil {
		return errUnavailable
	}
	var hashes []string
	for _, cookie := range r.CookiesNamed(sessionCookieName) {
		if validValue(cookie.Value) {
			hashes = append(hashes, hashToken(cookie.Value))
		}
	}
	row, err := db.CreatePasswordSession(ctx, store.PasswordSessionParams{Verified: verified, Session: store.CreateSessionParams{TokenHash: hashToken(token), CsrfSecret: secret, ExpiresAt: m.now().Add(idleLifetime), ReplacedHashes: hashes}})
	for _, hash := range append(hashes, row.RevokedHashes...) {
		rev := revocation{hash: hash}
		m.fence(rev)
		m.unfence(rev)
	}
	if err != nil {
		m.EvictAccount(verified.AccountID)
		return err
	}
	if ctx.Err() != nil {
		return errUnavailable
	}
	expires := minTime(row.ExpiresAt, row.CreatedAt.Add(absoluteLifetime))
	if !expires.After(m.now()) {
		return errUnavailable
	}
	setSessionCookie(w, token, expires, m.now())
	return nil
}
