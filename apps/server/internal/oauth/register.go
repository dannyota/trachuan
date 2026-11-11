package oauth

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"time"
)

// NewRegisterHandler creates the JSON registration endpoint. The caller wraps
// it in the shared login and OAuth IP limiter before mounting it.
func NewRegisterHandler(admit func(context.Context, Client) (Client, error)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "POST")
			writeClientError(w, 405, "invalid_client_metadata")
			return
		}
		kind, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || kind != "application/json" || len(r.Header.Values("Content-Type")) != 1 {
			writeClientError(w, 415, "invalid_client_metadata")
			return
		}
		raw, err := io.ReadAll(io.LimitReader(r.Body, 4097))
		if err != nil || len(raw) > 4096 {
			writeClientError(w, 400, "invalid_client_metadata")
			return
		}
		fields, err := metadataObject(raw)
		if err != nil {
			writeClientError(w, 400, "invalid_client_metadata")
			return
		}
		c, code := clientMetadata(fields, false)
		if code != "" {
			writeClientError(w, 400, code)
			return
		}
		c.ID, _, err = NewSecret(rand.Reader, "")
		if err != nil {
			writeClientError(w, 503, "temporarily_unavailable")
			return
		}
		c.Kind = "dcr"
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		c, err = admit(ctx, c)
		if err != nil {
			writeClientError(w, 503, "temporarily_unavailable")
			return
		}
		response := struct {
			ID        string   `json:"client_id"`
			IssuedAt  int64    `json:"client_id_issued_at"`
			Name      string   `json:"client_name"`
			Redirects []string `json:"redirect_uris"`
			Grants    []string `json:"grant_types"`
			Responses []string `json:"response_types"`
			Method    string   `json:"token_endpoint_auth_method"`
			Scope     string   `json:"scope"`
		}{c.ID, c.CreatedAt.Unix(), c.Name, c.RedirectURIs, []string{"authorization_code", "refresh_token"}, []string{"code"}, "none", "mcp:read"}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(response)
	})
}

func writeClientError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(struct {
		Error string `json:"error"`
	}{code})
}
