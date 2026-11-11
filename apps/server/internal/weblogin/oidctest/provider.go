// Package oidctest provides an ephemeral OIDC issuer for login tests.
package oidctest

import (
	"net/http/httptest"
	"testing"

	"danny.vn/trachuan/apps/server/internal/weblogin/oidcfake"
)

// ClientID is the test client's registered audience.
const ClientID = oidcfake.ClientID

// Claims describes an ID token.
type Claims = oidcfake.Claims

// Provider serves an ephemeral OIDC provider.
type Provider struct{ *oidcfake.Provider }

// New starts a provider and registers its cleanup with t.
func New(t *testing.T) *Provider {
	t.Helper()
	p, err := oidcfake.New()
	if err != nil {
		t.Fatal(err)
	}
	p.ReportError = func(err error) { t.Error(err) }
	s := httptest.NewServer(p)
	p.URL = s.URL
	t.Cleanup(s.Close)
	return &Provider{p}
}
