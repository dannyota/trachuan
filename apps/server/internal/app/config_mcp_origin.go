package app

import (
	"errors"
	"net/url"
	"os"
	"strconv"
	"strings"
)

func loadMCPOriginConfig() ([]string, string, error) {
	mode, present := os.LookupEnv("SERVER_MCP_ORIGIN_MODE")
	if !present {
		mode = "enforce"
	}
	if mode != "log" && mode != "enforce" {
		return nil, "", errors.New("invalid SERVER_MCP_ORIGIN_MODE")
	}
	var origins []string
	if raw := strings.TrimSpace(os.Getenv("SERVER_MCP_ORIGINS")); raw != "" {
		for _, part := range strings.Split(raw, ",") {
			origin := strings.TrimSpace(part)
			if !validMCPOrigin(origin) {
				return nil, "", errors.New("invalid SERVER_MCP_ORIGINS")
			}
			origins = append(origins, origin)
		}
	}
	return origins, mode, nil
}

func validMCPOrigin(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" || u.Hostname() == "" || u.User != nil || u.Opaque != "" || u.Path != "" || u.RawPath != "" || strings.ContainsAny(raw, "?#") || strings.HasSuffix(u.Host, ":") {
		return false
	}
	if port := u.Port(); port != "" {
		n, parseErr := strconv.Atoi(port)
		if parseErr != nil || n < 1 || n > 65535 {
			return false
		}
	}
	return true
}
