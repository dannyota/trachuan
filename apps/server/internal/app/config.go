package app

import (
	"errors"
	"net"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"

	"danny.vn/trachuan/apps/server/internal/oauth"
)

type config struct {
	listen, authURL, corpusURL, embedderURL, audience, publicURL, googleID, googleSecret, googleIssuer string
	strictDeployment                                                                                   bool
	trustedProxies                                                                                     []netip.Prefix
	oauthClients                                                                                       []oauth.Client
}

func loadConfig() (config, error) {
	c := config{listen: os.Getenv("SERVER_LISTEN"), authURL: os.Getenv("SERVER_AUTH_DATABASE_URL"), corpusURL: os.Getenv("SERVER_CORPUS_DATABASE_URL"), embedderURL: os.Getenv("SERVER_QUERY_EMBEDDER_URL")}
	if c.listen == "" {
		c.listen = "127.0.0.1:8080"
	}
	if c.embedderURL == "" {
		c.embedderURL = "http://127.0.0.1:8091"
	}
	var err error
	rawPublicURL := os.Getenv("SERVER_PUBLIC_URL")
	origin, ok := publicOrigin(rawPublicURL)
	c.googleIssuer = os.Getenv("SERVER_GOOGLE_ISSUER")
	if c.googleIssuer != "" && (!ok || !issuerAllowed(c.googleIssuer, rawPublicURL, c.listen)) {
		return config{}, errors.New("SERVER_GOOGLE_ISSUER requires a loopback deployment")
	}
	if c.authURL == "" || !validCorpusURL(c.corpusURL) || !ok {
		return config{}, errors.New("invalid or missing server configuration")
	}
	c.strictDeployment = loopbackDeployment(rawPublicURL, c.listen)
	c.publicURL = origin
	c.audience = origin + "/mcp"
	c.oauthClients, err = oauth.LoadPreregistered(os.Getenv("SERVER_OAUTH_CLIENTS"))
	if err != nil {
		return config{}, errors.New("invalid preregistered OAuth clients")
	}
	c.googleID = os.Getenv("GOOGLE_CLIENT_ID")
	c.googleSecret = os.Getenv("GOOGLE_CLIENT_SECRET")
	raw := strings.TrimSpace(os.Getenv("SERVER_TRUSTED_PROXIES"))
	if raw != "" {
		for _, part := range strings.Split(raw, ",") {
			p, err := netip.ParsePrefix(strings.TrimSpace(part))
			if err != nil {
				return config{}, errors.New("invalid or missing server configuration")
			}
			c.trustedProxies = append(c.trustedProxies, p.Masked())
		}
	}
	return c, nil
}

func publicOrigin(base string) (string, bool) {
	u, err := url.Parse(base)
	if err != nil || u == nil || u.Host == "" || u.Hostname() == "" || u.User != nil || u.Opaque != "" || (u.Path != "" && u.Path != "/") || u.RawPath != "" || strings.ContainsAny(base, "?#") || strings.HasSuffix(u.Host, ":") {
		return "", false
	}
	if u.Scheme != "https" && (u.Scheme != "http" || !loopback(u.Hostname())) {
		return "", false
	}
	host := strings.ToLower(u.Hostname())
	port := u.Port()
	if (u.Scheme == "https" && port == "443") || (u.Scheme == "http" && port == "80") {
		port = ""
	}
	if port != "" {
		host = net.JoinHostPort(host, port)
	} else if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	return u.Scheme + "://" + host, true
}
func loopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
func validCorpusURL(template string) bool {
	if strings.Count(template, "{database}") != 1 || (!strings.HasSuffix(template, "/{database}") && !strings.Contains(template, "/{database}?")) {
		return false
	}
	u, err := url.Parse(template)
	if err != nil || u == nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Host == "" || u.Path != "/{database}" {
		return false
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return false
	}
	_, overrides := q["dbname"]
	return !overrides
}

func strictLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip, err := netip.ParseAddr(host)
	return err == nil && ip.IsLoopback() && ip.Zone() == "" && !ip.Is4In6()
}

func loopbackDeployment(origin, listen string) bool {
	public, err := url.Parse(origin)
	if err != nil || !strictLoopback(public.Hostname()) {
		return false
	}
	host, _, err := net.SplitHostPort(listen)
	if err != nil || !strictLoopback(host) {
		return false
	}
	return true
}

func issuerAllowed(issuer, origin, listen string) bool {
	if !loopbackDeployment(origin, listen) {
		return false
	}
	u, err := url.Parse(issuer)
	if err != nil || u.Scheme != "http" || u.User != nil || u.Opaque != "" || u.Path != "" || u.RawPath != "" || strings.ContainsAny(issuer, "?#") || !strictLoopback(u.Hostname()) {
		return false
	}
	host := u.Hostname()
	ip, err := netip.ParseAddr(host)
	if err == nil {
		host = ip.String()
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil || port < 1 || port > 65535 || port == 80 {
		return false
	}
	return issuer == "http://"+net.JoinHostPort(host, strconv.Itoa(port))
}
