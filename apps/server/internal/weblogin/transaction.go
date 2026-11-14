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

type LoginTransaction struct {
	HandleHash, State, Nonce, PKCEVerifier, NextPath, RedirectURI string
	CreatedAt, ExpiresAt                                          time.Time
}

type ClaimLoginTransactionParams struct {
	HandleHash string
	Now        time.Time
}

type Store interface {
	CreateLoginTransaction(context.Context, LoginTransaction) error
	ClaimLoginTransaction(context.Context, ClaimLoginTransactionParams) (LoginTransaction, error)
}

func (h *Handler) begin(ctx context.Context, next string) (string, LoginTransaction, error) {
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
