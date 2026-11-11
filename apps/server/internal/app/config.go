package app

import (
	"errors"
	"net"
	"net/url"
	"os"
	"strings"
)

type config struct{ listen, authURL, corpusURL, embedderURL, audience string }

func loadConfig() (config, error) {
	c := config{listen: os.Getenv("SERVER_LISTEN"), authURL: os.Getenv("SERVER_AUTH_DATABASE_URL"), corpusURL: os.Getenv("SERVER_CORPUS_DATABASE_URL"), embedderURL: os.Getenv("SERVER_QUERY_EMBEDDER_URL")}
	if c.listen == "" {
		c.listen = "127.0.0.1:8080"
	}
	if c.embedderURL == "" {
		c.embedderURL = "http://127.0.0.1:8091"
	}
	audience, ok := publicAudience(os.Getenv("SERVER_PUBLIC_URL"))
	if c.authURL == "" || !validCorpusURL(c.corpusURL) || !ok {
		return config{}, errors.New("invalid or missing server configuration")
	}
	c.audience = audience
	return c, nil
}

func publicAudience(base string) (string, bool) {
	u, err := url.Parse(base)
	if err != nil || u == nil || u.Host == "" || u.Hostname() == "" || u.User != nil || u.Opaque != "" || (u.Path != "" && u.Path != "/") || u.RawPath != "" || strings.ContainsAny(base, "?#") || strings.HasSuffix(u.Host, ":") {
		return "", false
	}
	if u.Scheme != "https" && (u.Scheme != "http" || !loopback(u.Hostname())) {
		return "", false
	}
	return u.Scheme + "://" + u.Host + "/mcp", true
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
