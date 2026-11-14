package websession

import (
	"context"
	"errors"
	"net/http"

	"danny.vn/trachuan/apps/server/internal/store"
	"danny.vn/trachuan/apps/server/internal/weblogin"
)

type googleSessionStore interface {
	CreateGoogleSession(context.Context, store.GoogleSessionParams) (store.CreateSessionRow, error)
}

func (m *Manager) CreateGoogleSession(ctx context.Context, w http.ResponseWriter, r *http.Request, identity store.GoogleIdentity) error {
	ctx, cancel := context.WithTimeout(ctx, authQueryDeadline)
	defer cancel()
	db, ok := m.Store.(googleSessionStore)
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
	pendingParams := store.PendingParams{Next: weblogin.ValidateNext(identity.Next)}
	if identity.PendingHash != "" {
		pendingParams.ReplacedHashes = []string{identity.PendingHash}
	}
	row, err := db.CreateGoogleSession(ctx, store.GoogleSessionParams{Identity: identity, Pending: pendingParams, Session: store.CreateSessionParams{TokenHash: hashToken(token), CsrfSecret: secret, ExpiresAt: m.now().Add(idleLifetime)}, ReplacedHashes: hashes})
	for _, hash := range append(hashes, row.RevokedHashes...) {
		rev := revocation{hash: hash}
		m.fence(rev)
		m.unfence(rev)
	}
	if err != nil {
		var pending *store.PendingRequired
		if errors.As(err, &pending) {
			if ctx.Err() != nil {
				return errUnavailable
			}
			setPending(w, token, pendingAge())
			return err
		}
		if row.AccountID != 0 {
			m.EvictAccount(row.AccountID)
		}
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
	if identity.PendingHash != "" || len(r.CookiesNamed("__Host-pending")) > 0 {
		ClearPending(w)
	}
	return nil
}
