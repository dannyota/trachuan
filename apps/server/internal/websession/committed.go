package websession

import (
	"context"
	"net/http"

	"danny.vn/trachuan/apps/server/internal/store"
)

func (m *Manager) CommitSession(ctx context.Context, w http.ResponseWriter, r *http.Request, write func(context.Context, store.CreateSessionParams) (store.CreateSessionRow, error)) error {
	ctx, cancel := context.WithTimeout(ctx, authQueryDeadline)
	defer cancel()
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
	row, err := write(ctx, store.CreateSessionParams{TokenHash: hashToken(token), CsrfSecret: secret, ExpiresAt: m.now().Add(idleLifetime), ReplacedHashes: hashes})
	if row.AccountID != 0 {
		m.EvictAccount(row.AccountID)
	}
	for _, digest := range append(hashes, row.RevokedHashes...) {
		rev := revocation{hash: digest}
		m.fence(rev)
		m.unfence(rev)
	}
	if err != nil {
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
