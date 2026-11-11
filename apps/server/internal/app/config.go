package app

import (
	"errors"
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
	base := os.Getenv("SERVER_PUBLIC_URL")
	u, err := url.Parse(base)
	if c.authURL == "" || c.corpusURL == "" || err != nil || u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return config{}, errors.New("invalid or missing server configuration")
	}
	c.audience = strings.TrimRight(base, "/") + "/mcp"
	return c, nil
}
