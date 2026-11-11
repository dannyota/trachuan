// Package fakegoogle provides the loopback test identity provider.
package fakegoogle

import (
	"errors"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"net/netip"
	"strconv"

	"danny.vn/trachuan/apps/server/internal/weblogin/oidcfake"
)

// Config configures the test identity provider.
type Config struct{ Issuer, ClientID, ClientSecret, RedirectURI string }
type provider struct {
	config Config
	core   *oidcfake.Provider
	form   *template.Template
}

// New creates a test identity provider with a fresh signing key.
func New(c Config) (http.Handler, error) {
	if c.Issuer == "" || c.ClientID == "" || c.ClientSecret == "" || c.RedirectURI == "" {
		return nil, errors.New("missing provider configuration")
	}
	core, err := oidcfake.New()
	if err != nil {
		return nil, fmt.Errorf("create provider core: %w", err)
	}
	core.URL = c.Issuer
	form, err := template.New("authorize").Parse(formHTML)
	if err != nil {
		return nil, fmt.Errorf("parse authorization form: %w", err)
	}
	return &provider{config: c, core: core, form: form}, nil
}

// ValidListen reports whether addr has a strict loopback host and a valid port.
func ValidListen(addr string) bool {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	for _, digit := range port {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip, err := netip.ParseAddr(host)
	return err == nil && ip.IsLoopback() && ip.Zone() == "" && !ip.Is4In6()
}

func (p *provider) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/authorize":
		p.authorize(w, r)
	case "/token":
		if r.Method != http.MethodPost || r.ParseForm() != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		id, secret, ok := r.BasicAuth()
		if !ok {
			id, secret = r.PostForm.Get("client_id"), r.PostForm.Get("client_secret")
		}
		if id != p.config.ClientID || secret != p.config.ClientSecret {
			http.Error(w, "invalid client", http.StatusBadRequest)
			return
		}
		p.core.ServeHTTP(w, r)
	default:
		p.core.ServeHTTP(w, r)
	}
}
