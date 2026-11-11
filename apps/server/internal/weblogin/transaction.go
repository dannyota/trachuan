// Package weblogin implements Google login with single-use transactions.
package weblogin

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"time"

	"golang.org/x/oauth2"
)

// LoginTransaction holds the server-side authorization request and handle hash.
type LoginTransaction struct {
	HandleHash, State, Nonce, PKCEVerifier, NextPath, RedirectURI string
	CreatedAt, ExpiresAt                                          time.Time
}

// ClaimLoginTransactionParams identifies one transaction and the claim time.
type ClaimLoginTransactionParams struct {
	HandleHash string
	Now        time.Time
}

// UpsertGoogleAccountParams identifies an account by subject and current email.
type UpsertGoogleAccountParams struct{ Sub, Email string }

// GoogleAccount contains the login eligibility returned by the account upsert.
type GoogleAccount struct {
	ID                       int64
	Blocked, DeleteRequested bool
}

// Store persists login state. ClaimLoginTransaction must atomically claim only
// an unclaimed row whose expiry is after Now and return sql.ErrNoRows otherwise.
// HandleHash is lowercase SHA-256 hex. UpsertGoogleAccount keys only by Sub and
// updates current email and last_login_at, returning both account block flags.
type Store interface {
	CreateLoginTransaction(context.Context, LoginTransaction) error
	ClaimLoginTransaction(context.Context, ClaimLoginTransactionParams) (LoginTransaction, error)
	UpsertGoogleAccount(context.Context, UpsertGoogleAccountParams) (GoogleAccount, error)
}

func (h *Handler) begin(ctx context.Context, next string) (string, LoginTransaction, error) {
	// Readers injected by tests need not be safe for concurrent starts.
	h.randomMu.Lock()
	handle, state, nonce, err := h.transactionValues()
	h.randomMu.Unlock()
	if err != nil {
		return "", LoginTransaction{}, err
	}
	now := h.clock()
	tx := LoginTransaction{HandleHash: hashHandle(handle), State: state, Nonce: nonce, PKCEVerifier: oauth2.GenerateVerifier(), NextPath: next, RedirectURI: h.oauth.RedirectURL, CreatedAt: now, ExpiresAt: now.Add(transactionTTL)}
	if err := h.store.CreateLoginTransaction(ctx, tx); err != nil {
		return "", LoginTransaction{}, fmt.Errorf("create login transaction: %w", err)
	}
	return handle, tx, nil
}

func (h *Handler) transactionValues() (handle, state, nonce string, err error) {
	handle, err = randomValue(h.random)
	if err != nil {
		return
	}
	state, err = randomValue(h.random)
	if err != nil {
		return
	}
	nonce, err = randomValue(h.random)
	return
}

func hashHandle(handle string) string {
	sum := sha256.Sum256([]byte(handle))
	return hex.EncodeToString(sum[:])
}

func encodeValue(value []byte) string { return base64.RawURLEncoding.EncodeToString(value) }

func validHandle(handle string) bool {
	if len(handle) != 43 {
		return false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(handle)
	return err == nil && len(decoded) == 32 && encodeValue(decoded) == handle
}
