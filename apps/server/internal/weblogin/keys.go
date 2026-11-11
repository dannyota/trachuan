package weblogin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	jose "github.com/go-jose/go-jose/v4"
)

// newVerifier retains go-oidc's shared key cache while associating a key-fetch
// failure with each verification, even when several callers share a fetch.
func newVerifier(ctx context.Context, provider *oidc.Provider, issuer, clientID string, clock func() time.Time, client *http.Client) (*oidc.IDTokenVerifier, error) {
	var metadata struct {
		JWKSURL    string   `json:"jwks_uri"`
		Algorithms []string `json:"id_token_signing_alg_values_supported"`
	}
	if err := provider.Claims(&metadata); err != nil {
		return nil, fmt.Errorf("read discovery metadata: %w", err)
	}
	if metadata.JWKSURL == "" {
		return nil, errors.New("missing provider keys URL")
	}
	var algorithms []string
	for _, alg := range metadata.Algorithms {
		if supportedAlgorithm(alg) {
			algorithms = append(algorithms, alg)
		}
	}
	transport := client.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	keyClient := *client
	keyClient.Transport = keyTransport{base: transport}
	remote := oidc.NewRemoteKeySet(oidc.ClientContext(ctx, &keyClient), metadata.JWKSURL)
	return oidc.NewVerifier(issuer, trackedKeySet{remote: remote}, &oidc.Config{ClientID: clientID, Now: clock, SupportedSigningAlgs: algorithms}), nil
}

func supportedAlgorithm(alg string) bool {
	switch alg {
	case oidc.RS256, oidc.RS384, oidc.RS512, oidc.ES256, oidc.ES384, oidc.ES512, oidc.PS256, oidc.PS384, oidc.PS512, oidc.EdDSA:
		return true
	default:
		return false
	}
}

type keyFailureContextKey struct{}

type trackedKeySet struct{ remote *oidc.RemoteKeySet }

func (s trackedKeySet) VerifySignature(ctx context.Context, token string) ([]byte, error) {
	payload, err := s.remote.VerifySignature(ctx, token)
	var unavailable keyUnavailableError
	if errors.As(err, &unavailable) {
		if failure, ok := ctx.Value(keyFailureContextKey{}).(*bool); ok {
			*failure = true
		}
	}
	return payload, err
}

// keyUnavailableError crosses go-oidc's remote-key cache without exposing the
// provider's response. The outer verifier does not preserve wrapped errors.
type keyUnavailableError struct{ cause error }

func (e keyUnavailableError) Error() string { return "provider keys unavailable" }
func (e keyUnavailableError) Unwrap() error { return e.cause }

type keyTransport struct{ base http.RoundTripper }

func (t keyTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	response, err := t.base.RoundTrip(r)
	if err != nil {
		return nil, keyUnavailableError{cause: err}
	}
	if response.StatusCode != http.StatusOK {
		_ = response.Body.Close()
		return nil, keyUnavailableError{}
	}
	data, readErr := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	closeErr := response.Body.Close()
	if readErr != nil || closeErr != nil || len(data) > 1<<20 || !validKeySet(data) {
		return nil, keyUnavailableError{}
	}
	response.Body = io.NopCloser(bytes.NewReader(data))
	return response, nil
}

func validKeySet(data []byte) bool {
	var set struct {
		Keys []json.RawMessage `json:"keys"`
	}
	if err := json.Unmarshal(data, &set); err != nil || set.Keys == nil {
		return false
	}
	for _, raw := range set.Keys {
		var metadata struct {
			Algorithm string `json:"alg"`
		}
		if err := json.Unmarshal(raw, &metadata); err != nil {
			return false
		}
		if metadata.Algorithm != "" && !supportedAlgorithm(metadata.Algorithm) {
			continue
		}
		var key jose.JSONWebKey
		if err := json.Unmarshal(raw, &key); err != nil && !errors.Is(err, jose.ErrUnsupportedKeyType) {
			return false
		}
	}
	return true
}
