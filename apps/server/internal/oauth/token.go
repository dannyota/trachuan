package oauth

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"danny.vn/trachuan/apps/server/internal/store"
	"github.com/jackc/pgx/v5"
)

type TokenDB interface {
	Begin(context.Context) (pgx.Tx, error)
}

type tokenHandler struct {
	db              TokenDB
	resource        string
	invalidateGrant func(int64)
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	Scope        string `json:"scope"`
}

func NewTokenHandler(db TokenDB, base string, invalidateGrant func(int64)) http.Handler {
	return &tokenHandler{db: db, resource: base + "/mcp", invalidateGrant: invalidateGrant}
}

func (h *tokenHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if len(r.Header.Values("Authorization")) != 0 {
		writeTokenClientAuthError(w)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		writeTokenError(w, 405, "invalid_request")
		return
	}
	form, code := parseTokenForm(r, h.resource)
	if code != "" {
		if code == "invalid_client" {
			writeTokenClientAuthError(w)
			return
		}
		writeTokenError(w, 400, code)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	tx, err := h.db.Begin(ctx)
	if err != nil {
		writeTokenError(w, 503, "temporarily_unavailable")
		return
	}
	defer func() {
		rollbackCtx, rollbackCancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer rollbackCancel()
		_ = tx.Rollback(rollbackCtx)
	}()
	clientRow, err := store.New(tx).GetClient(ctx, form.Get("client_id"))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeTokenError(w, 400, "invalid_client")
		} else {
			writeTokenError(w, 503, "temporarily_unavailable")
		}
		return
	}
	if !clientFromRow(clientRow).Allows(form.Get("grant_type")) {
		writeTokenError(w, 400, "unauthorized_client")
		return
	}
	var response tokenResponse
	var invalidateID int64
	if form.Get("grant_type") == "authorization_code" {
		response, invalidateID, code, err = h.exchangeCode(ctx, tx, form)
	} else {
		response, invalidateID, code, err = h.refresh(ctx, tx, form)
	}
	if err != nil {
		writeTokenError(w, 503, "temporarily_unavailable")
		return
	}
	if code != "" && invalidateID == 0 {
		writeTokenError(w, 400, code)
		return
	}
	err = tx.Commit(ctx)
	if invalidateID != 0 && h.invalidateGrant != nil {
		h.invalidateGrant(invalidateID)
	}
	if err != nil {
		writeTokenError(w, 503, "temporarily_unavailable")
		return
	}
	if code != "" {
		writeTokenError(w, 400, code)
		return
	}
	tokenHeaders(w)
	_ = json.NewEncoder(w).Encode(response)
}

func writeTokenClientAuthError(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Basic realm="trachuan"`)
	writeTokenError(w, http.StatusUnauthorized, "invalid_client")
}

func tokenHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
}

func writeTokenError(w http.ResponseWriter, status int, code string) {
	tokenHeaders(w)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(struct {
		Error string `json:"error"`
	}{code})
}

func lockTokenAuthority(ctx context.Context, tx pgx.Tx, accountID, grantID int64, clientID, resource, grantType string) (int32, Client, error) {
	account, err := store.LockAccount(ctx, tx, accountID)
	if err != nil {
		return 0, Client{}, fmt.Errorf("lock token account: %w", err)
	}
	if account.BlockedAt != nil || account.DeleteRequestedAt != nil {
		return 0, Client{}, pgx.ErrNoRows
	}
	client, err := InUse(ctx, tx, clientID)
	if errors.Is(err, ErrInvalidClient) {
		return 0, Client{}, tokenAuthorityFailure("invalid_client")
	}
	if err != nil {
		return 0, Client{}, err
	}
	if !client.Allows(grantType) {
		return 0, Client{}, tokenAuthorityFailure("unauthorized_client")
	}
	grant, err := store.New(tx).LockGrantAuthority(ctx, grantID)
	if err != nil {
		return 0, Client{}, fmt.Errorf("lock token grant: %w", err)
	}
	if grant.RevokedAt != nil || grant.AccountID != account.ID || grant.ClientID != clientID || grant.Resource != resource || grant.Scope != "mcp:read" {
		return 0, Client{}, pgx.ErrNoRows
	}
	return account.AuthEpoch, client, nil
}

type tokenAuthorityFailure string

func (e tokenAuthorityFailure) Error() string { return string(e) }

func tokenStateError(err error) (string, error) {
	var authority tokenAuthorityFailure
	if errors.As(err, &authority) {
		return string(authority), nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return "invalid_grant", nil
	}
	return "", err
}

func issueTokenPair(ctx context.Context, tx pgx.Tx, params store.CreateTokenFamilyParams, issueRefresh bool) (tokenResponse, error) {
	access, accessHash, err := NewSecret(rand.Reader, "tra_")
	if err != nil {
		return tokenResponse{}, fmt.Errorf("create access token: %w", err)
	}
	now := time.Now()
	accessExpiry := now.Add(time.Hour)
	if !issueRefresh {
		params.FamilyExpiresAt = accessExpiry
	}
	if params.FamilyExpiresAt.Before(accessExpiry) {
		accessExpiry = params.FamilyExpiresAt
	}
	params.TokenHash = fmt.Sprintf("%x", accessHash)
	params.Kind = "access"
	params.ExpiresAt = accessExpiry
	predecessor := params.RotatedFrom
	params.RotatedFrom = nil
	if _, err = store.New(tx).CreateTokenFamily(ctx, params); err != nil {
		return tokenResponse{}, fmt.Errorf("store access token: %w", err)
	}
	response := tokenResponse{AccessToken: access, TokenType: "Bearer", ExpiresIn: int64(accessExpiry.Sub(now) / time.Second), Scope: params.Scope}
	if !issueRefresh {
		return response, nil
	}
	refresh, refreshHash, err := NewSecret(rand.Reader, "trr_")
	if err != nil {
		return tokenResponse{}, fmt.Errorf("create refresh token: %w", err)
	}
	params.TokenHash = fmt.Sprintf("%x", refreshHash)
	params.Kind = "refresh"
	params.ExpiresAt = params.FamilyExpiresAt
	params.RotatedFrom = predecessor
	if _, err = store.New(tx).CreateTokenFamily(ctx, params); err != nil {
		return tokenResponse{}, fmt.Errorf("store refresh token: %w", err)
	}
	response.RefreshToken = refresh
	return response, nil
}
